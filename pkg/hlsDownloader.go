package HLSDownloader

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Incrementer interface {
	Increment()
}

const defaultWorkers = 5

type hlsDownloader struct {
	url       string
	output    string
	path      string
	filename  string
	extension string
	tmpDir    string

	client *http.Client
	header *http.Header

	workers int
	bar     Incrementer
}

func New(URL string, output string) (*hlsDownloader, error) {
	out, err := validateParameters(URL, output)
	if err != nil {
		return nil, err
	}
	return &hlsDownloader{
		header: &http.Header{},
		client: &http.Client{},

		url: URL,

		output:    out.output,
		path:      out.path,
		filename:  out.filename,
		extension: out.extension,

		workers: defaultWorkers,

		bar: nil,
	}, nil
}

func (h *hlsDownloader) SetClient(client *http.Client) error {
	if h == nil {
		return errors.New("attempt to set client on nil instance")
	}
	h.client = client
	return nil
}
func (h *hlsDownloader) SetHeader(header *http.Header) error {
	if h == nil {
		return errors.New("attempt to set header on nil instance")
	}
	h.header = header
	return nil
}
func (h *hlsDownloader) SetWorkers(workers int) error {
	if h == nil {
		return errors.New("attempt to set workers on nil instance")
	}
	if workers < 1 {
		return errors.New("workers must be greater than 0")
	}
	h.workers = workers
	return nil
}
func (h *hlsDownloader) SetBar(bar *Incrementer) error {
	if h == nil {
		return errors.New("attempt to set bar on nil instance")
	}
	h.bar = *bar
	return nil
}

func (h *hlsDownloader) Download() (string, error) {
	if h == nil {
		return "", errors.New("instance is nil")
	}
	segments, err := parseHLSSegments(h.url, h.header)
	log.Printf("Total Segments: %d", len(segments))
	if err != nil {
		return "", err
	}

	err = os.MkdirAll(h.path, os.ModePerm)
	if err != nil {
		return "", err
	}
	h.tmpDir, err = os.MkdirTemp("", "*-segments")
	log.Printf("Temp Dir: %s", h.tmpDir)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(h.tmpDir)

	err = h.processSegments(segments)
	if err != nil {
		return "", err
	}

	filepath, err := h.join(segments)
	if err != nil {
		return "", err
	}

	return filepath, nil
}

func (h *hlsDownloader) join(segments []*segment) (string, error) {
	file, err := os.Create(h.output)
	if err != nil {
		return "", err
	}
	defer file.Close()

	sort.Slice(segments, func(i, j int) bool {
		return segments[i].SeqId < segments[j].SeqId
	})

	for _, segment := range segments {

		d, err := decrypt(segment, h.client)
		if err != nil {
			return "", err
		}

		if _, err := file.Write(d); err != nil {
			return "", err
		}

		if err := os.RemoveAll(segment.path); err != nil {
			return "", err
		}
	}
	log.Printf("Joined segments into %s", h.output)
	return h.output, nil
}

func (h *hlsDownloader) downloadSegment(segment *segment) error {
	req, err := newRequest(segment.URI, h.header)
	if err != nil {
		return err
	}

	res, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return errors.New(res.Status)
	}

	file, err := os.Create(segment.path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, res.Body)
	if err != nil {
		return err
	}
	return nil
}

func (h *hlsDownloader) downloadSegments(wc *workerController) {
	defer wc.wg.Done()
	maxAttempts := 3
	for segment := range wc.segments {
		attempts := 0
		for {
			if h.isAbort(wc) {
				close(wc.downloadResult)
				return
			}
			err := h.downloadSegment(segment)
			if err == nil {
				log.Printf("Downloaded segment %d\n", segment.SeqId)
				wc.downloadResult <- &downloadResult{seqId: segment.SeqId}
				break
			}
			connectionReset := strings.Contains(err.Error(), "connection reset by peer")
			if connectionReset && attempts < maxAttempts {
				attempts++
				time.Sleep(time.Second)
				log.Printf("Connection reset by peer, retrying download of segment %d. Attempt #%d\n", segment.SeqId, attempts)
				continue
			}
			log.Printf("Error downloading segment %d: %s\n", segment.SeqId, err.Error())
			wc.downloadResult <- &downloadResult{err: err, seqId: segment.SeqId}
			break
		}
	}
}

func (h *hlsDownloader) isAbort(wc *workerController) bool {
	select {
	case <-wc.abort:
		log.Printf("Abort signal received\n")
		return true
	default:
	}
	return false
}

func (h *hlsDownloader) prepareSegments(segments []*segment, wc *workerController) {
	defer close(wc.segments)
	for _, segment := range segments {
		if h.isAbort(wc) {
			return
		}
		segName := fmt.Sprintf("seg%d.ts", segment.SeqId)
		segment.path = filepath.Join(h.tmpDir, segName)
		wc.segments <- segment
	}
}

func (h *hlsDownloader) processSegments(segments []*segment) error {
	wc := &workerController{
		wg:             sync.WaitGroup{},
		segments:       make(chan *segment),
		downloadResult: make(chan *downloadResult),
		abort:          make(chan struct{}),
		success:        make(chan struct{}),
	}

	for i := 0; i < h.workers; i++ {
		wc.wg.Add(1)
		go h.downloadSegments(wc)
	}
	go h.prepareSegments(segments, wc)

	go func() {
		wc.wg.Wait()
		wc.success <- struct{}{}
	}()

	for {
		select {
		case <-wc.success:
			return nil
		case result := <-wc.downloadResult:
			if result.err != nil {
				close(wc.abort)
				return result.err
			}
			if h.bar != nil {
				h.bar.Increment()
			}
		}
	}
}
