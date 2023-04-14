package HLSDownloader

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/grafov/m3u8"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type segment struct {
	*m3u8.MediaSegment
	path string
}

type downloadResult struct {
	err           error
	seqId         uint64
	totalSegments uint64
}

type workerController struct {
	segments       chan *segment
	downloadResult chan *downloadResult
	abort          chan struct{}
	success        chan struct{}
	wg             sync.WaitGroup
}

type outParams struct {
	output    string
	path      string
	filename  string
	extension string
}

const (
	syncByte = uint8(71) //0x47
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func testFileWrite(out outParams) error {
	file, err := os.Create(out.output)
	if err != nil {
		if os.IsPermission(err) {
			return os.ErrPermission
		}
	}
	defer os.Remove(out.output)
	defer file.Close()
	return nil
}

func validateOutputPermission(out outParams) error {
	info, err := os.Stat(out.output)
	if err != nil {
		if os.IsNotExist(err) {
			return testFileWrite(out)
		}
	}
	if info == nil {
		return errors.New("output path is not valid")
	}
	if info.IsDir() {
		if info.Mode().Perm()&(1<<7) == 0 {
			return os.ErrPermission
		}
	} else {
		if !info.Mode().IsRegular() {
			return os.ErrNotExist
		}

		if info.Mode().Perm()&0200 == 0 {
			return os.ErrPermission
		}
		return testFileWrite(out)
	}
	return nil
}

func validateOutput(output string) (outParams, error) {
	var err error
	now := time.Now().Unix()
	nowFilename := fmt.Sprintf("%d.ts", now)

	if output == "" {
		log.Printf("No output file specified, saving to current directory as %s\n", nowFilename)
		output, err = os.Getwd()
		if err != nil {
			return outParams{}, err
		}
	}
	path, filename := filepath.Split(output)
	if path == "" {
		path, err = os.Getwd()
		if err != nil {
			return outParams{}, err
		}
	}

	if filename == "" {
		filename = nowFilename
	}
	extension := filepath.Ext(filename)
	if extension == "" {
		output += ".ts"
		filename += ".ts"
		extension = ".ts"
	}
	output = filepath.Join(path, filename)
	_, err = os.Stat(output)
	if err != nil {
		inputParams := outParams{
			output:    output,
			path:      path,
			filename:  filename,
			extension: extension,
		}
		return inputParams, nil
	}
	log.Printf("File %s already exists\n", filename)
	filename = fmt.Sprintf("%d%s", time.Now().Unix(), extension)
	output = filepath.Join(path, filename)
	log.Printf("Saving file as %s instead\n", filename)
	return validateOutput(output)
}

func validateURL(URL string) error {
	resp, err := http.Head(URL)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		errorMessage := fmt.Sprintf("url is not valid. %s", resp.Status)
		return errors.New(errorMessage)
	}
	return nil
}

func validateParameters(URL string, output string) (outParams, error) {
	err := validateURL(URL)
	if err != nil {
		return outParams{}, err
	}
	out, err := validateOutput(output)
	if err != nil {
		return out, err
	}
	err = validateOutputPermission(out)
	if err != nil {
		return out, err
	}

	return out, nil
}

func newRequest(url string, header *http.Header) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header = *header
	return req, nil
}

func getM3u8ListType(url string, header *http.Header) (m3u8.Playlist, m3u8.ListType, error) {

	req, err := newRequest(url, header)
	if err != nil {
		return nil, 0, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, 0, errors.New(res.Status)
	}

	p, t, err := m3u8.DecodeFrom(res.Body, false)
	if err != nil {
		return nil, 0, err
	}

	return p, t, nil
}

func parseHLSSegments(URL string, header *http.Header) ([]*segment, error) {
	baseURL, err := url.Parse(URL)
	if err != nil {
		return nil, errors.New("invalid url")
	}

	p, t, err := getM3u8ListType(URL, header)
	if err != nil {
		return nil, err
	}
	if t != m3u8.MEDIA {
		return nil, errors.New("M38U is not media type")
	}

	mediaList := p.(*m3u8.MediaPlaylist)
	var segments []*segment
	for _, seg := range mediaList.Segments {
		if seg == nil {
			continue
		}

		if !strings.Contains(seg.URI, "http") {
			segmentURL, err := baseURL.Parse(seg.URI)
			if err != nil {
				return nil, err
			}

			seg.URI = segmentURL.String()
		}

		if seg.Key == nil && mediaList.Key != nil {
			seg.Key = mediaList.Key
		}

		if seg.Key != nil && !strings.Contains(seg.Key.URI, "http") {
			keyURL, err := baseURL.Parse(seg.Key.URI)
			if err != nil {
				return nil, err
			}

			seg.Key.URI = keyURL.String()
		}

		segment := &segment{MediaSegment: seg}
		segments = append(segments, segment)
	}

	return segments, nil
}

func decryptAES128(crypted, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	blockMode := cipher.NewCBCDecrypter(block, iv[:blockSize])
	origData := make([]byte, len(crypted))
	blockMode.CryptBlocks(origData, crypted)
	origData = pkcs5UnPadding(origData)
	return origData, nil
}

func pkcs5UnPadding(origData []byte) []byte {
	length := len(origData)
	unPadding := int(origData[length-1])
	return origData[:(length - unPadding)]
}

func decrypt(segment *segment, client *http.Client) ([]byte, error) {

	file, err := os.Open(segment.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	if segment.Key != nil {
		key, iv, err := getKey(segment, client)
		if err != nil {
			return nil, err
		}
		data, err = decryptAES128(data, key, iv)
		if err != nil {
			return nil, err
		}
	}

	for j := 0; j < len(data); j++ {
		if data[j] == syncByte {
			data = data[j:]
			break
		}
	}

	return data, nil
}

func getKey(segment *segment, client *http.Client) (key []byte, iv []byte, err error) {
	res, err := client.Get(segment.Key.URI)
	if err != nil {
		return nil, nil, err
	}

	if res.StatusCode != 200 {
		return nil, nil, errors.New("Failed to get descryption key")
	}

	key, err = io.ReadAll(res.Body)
	if err != nil {
		return nil, nil, err
	}

	iv = []byte(segment.Key.IV)
	if len(iv) == 0 {
		iv = defaultIV(segment.SeqId)
	}
	return
}

func defaultIV(seqID uint64) []byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[8:], seqID)
	return buf
}
