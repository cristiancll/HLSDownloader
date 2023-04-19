# HLS downloader
This is a library to support downloading a m3u8 file. 

All ts segments will be downloaded into a temporary folder then be joined into the output file.

If no output file is specified, the default file name will be a random number with `.ts` extension

If no output folder is specified, the default folder will be the current directory


### Features:
* Concurrent download segments with multiple http connections
* Decrypt hls encoded segments
* Auto retry download
* Support for progress bars
* Support for custom HTTP Headers
* Support for custom HTTP Client

### How to integrate this library to your code.

Get the library
```
go get github.com/cristiancll/hlsdownloader
```
then import it to your code.
```
import "github.com/cristiancll/hlsdownloader"
```

Sample:

```go
package main

import (
	"github.com/cristiancll/hlsdownloader"
	"log"
	"net/http"
)

func main() {
    URL := "https://domain.com/path/to/file.m3u8"
    output := "C:\\path\\to\\output\\file.ts" // or just "C:\path\to\output\"

    hls, err := hlsDownloader.New(URL, output)
    if err != nil {
        log.Printf("Error creating HLSDownloader: %v\n", err)
        return
    }
    // If you want to use a custom http client
    httpClient := &http.Client{}
    hls.SetClient(httpClient)
	
    // If you want to use a custom http header
    httpHeader := &http.Header{}
    hls.SetHeader(httpHeader)
	
    // If you want to use a custom number of workers (default is 5)
    workers := 5
    hls.SetWorkers(workers) 
	
    _, err = hls.Download()
    if err != nil {
        log.Printf("Error downloading file: %v\n", err)
        return
    }
}


```
## Binaries
You are free to [download the binaries](https://github.com/cristiancll/HLSDownloader/releases) or build it yourself.

## Build

### Requirements
- You need to have [Go](https://golang.org/) installed on your machine (version 1.20 or higher)

```
cd path/to/HLSDownloader/cmd
go build -o ../bin/HLSDownloader.exe
```

## Usage

Run the binary with `--help` or `-h` to see the available options.

```
HLSDownloader.exe --help
```

### Available Commands
    
```
  -h    
        Show help
  -help 
        Show this help menu with all the available options
  -o string
        Path or Output file
  -output string
        The path to the folder or the output file itself that the m3u8 will be saved
  -u string
        Target URL
  -url string
        A http URL of the HLS stream/m3u8 file to be downloaded
  -w int
        Total Workers (default 5)
  -workers int
        The number of workers to be used simultaneously to download the file (default 5) (default 5)
```

Example:
```
HLSDownloader.exe -u https://domain.com/path/to/file.m3u8 -w 10 -u C:\path\to\output\file.ts
```


## Disclaimer

This project is based on [canhlinh/hlsdl](https://github.com/canhlinh/hlsdl), I just adjusted it to my needs.

