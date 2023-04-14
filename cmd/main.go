package main

import (
	"errors"
	"flag"
	"hlsDownloader/pkg"
	"log"
	"os"
)

func handleArgs() (string, string, int, error) {
	var URL string
	flag.StringVar(&URL, "url", "", "A http url of the HLS stream/m3u8 file to be downloaded")
	if URL == "" {
		flag.StringVar(&URL, "u", "", "Target url")
	}

	var output string
	flag.StringVar(&output, "output", "", "The path to the folder or the output file itself that the m3u8 will be saved")
	if output == "" {
		flag.StringVar(&output, "o", "", "Path or Output file")
	}

	var workers int
	flag.IntVar(&workers, "workers", 5, "The number of workers to be used simultaneously to download the file (default 5)")
	if workers == 5 {
		flag.IntVar(&workers, "w", 5, "Total Workers")
	}

	helpCmd := flag.Bool("help", false, "Show this help menu with all the available options")
	hCmd := flag.Bool("h", false, "Show help")

	flag.Parse()

	if *helpCmd || *hCmd {
		flag.PrintDefaults()
		os.Exit(0)
	}

	if URL == "" {
		return "", "", 0, errors.New("No url specified")
	}
	return URL, output, workers, nil
}

func main() {
	URL, output, workers, err := handleArgs()
	if err != nil {
		log.Printf("Invalid arguments: %v\n", err)
		return
	}

	hls, err := hlsDownloader.New(URL, output)
	if err != nil {
		log.Printf("Error creating hlsDownloader: %v\n", err)
		return
	}
	if workers > 0 {
		err := hls.SetWorkers(workers)
		if err != nil {
			log.Printf("Error setting workers: %v\n", err)
			return
		}
	}

	_, err = hls.Download()
	if err != nil {
		log.Printf("Error downloading file: %v\n", err)
		return
	}
}
