package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
	"github.com/imacks/cowtransfer"
)

var (
	blockSize int
	maxThreads int
	timeout time.Duration
	maxRetry int
	verifyHash bool
	uploadPassword string
	useragent string
	cookieToken string
)

func init() {
	flag.IntVar(&blockSize, "b", 262144, "Block size for uploading")
	flag.IntVar(&maxThreads, "p", 1, "Number of concurrent threads")
	flag.IntVar(&maxRetry, "r", 4, "Max failure retry")
	flag.BoolVar(&verifyHash, "S", false, "Verify hash for every block")
	flag.StringVar(&uploadPassword, "w", "", "Upload password")
	flag.StringVar(&useragent, "u", "", "Useragent string")
	flag.StringVar(&cookieToken, "W", "", "Custom cookie token pattern")
	flag.DurationVar(&timeout, "t", 10*time.Second, "Timeout duration")

	flag.Usage = func() {
		fmt.Fprintf(os.Stdout, "%s %s (%s) %s\n", AppName, Version, GitCommit, AppDesc)
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintf(os.Stdout, "Usage: %s [optional] file1 file2... \n", os.Args[0])
		fmt.Fprintf(os.Stdout, "       %s [optional] url\n", os.Args[0])
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintln(os.Stdout, "Parameters:")
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "no file specified!\n")
		os.Exit(1)
	}

	if len(files) == 1 && (strings.HasPrefix(files[0], "https://") || strings.HasPrefix(files[0], "http://")) {
		err := listRemoteFiles(files[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	err := uploadFiles(files)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func uploadFiles(files []string) error {
	for _, v := range files {
		if strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "http://") {
			return fmt.Errorf("upload supports local file path only: %s", v)
		}
	}

	if maxRetry < 0 {
		return fmt.Errorf("max retry must be at least 0")
	}
	if blockSize > 4194304 {
		return fmt.Errorf("block size out of range")
	}
	if maxThreads < 1 {
		return fmt.Errorf("max retry must be bigger than 0")
	}

	cc := cowtransfer.NewClient()
	cc.Timeout = timeout
	cc.MaxRetry = maxRetry
	cc.VerifyHash = verifyHash
	cc.BlockSize = blockSize
	cc.MaxPushBlocks = maxThreads

	if uploadPassword != "" {
		cc.Password = uploadPassword
	}
	if useragent != "" {
		cc.UserAgent = useragent
	}
	if cookieToken != "" {
		cc.Token = cookieToken
	}

	cc.OnStart(func(s *cowtransfer.UploadSession) {
		fmt.Fprintf(os.Stdout, "event: session_start\n")
		fmt.Fprintf(os.Stdout, "upload_token: %s\n", s.UploadToken)
		fmt.Fprintf(os.Stdout, "transfer_guid: %s\n", s.TransferGUID)
		fmt.Fprintf(os.Stdout, "file_guid: %s\n", s.FileGUID)
		fmt.Fprintf(os.Stdout, "url: %s\n", s.UniqueURL)
		fmt.Fprintf(os.Stdout, "prefix: %s\n", s.Prefix)
		fmt.Fprintf(os.Stdout, "qrcode: %s\n", s.QRCode)
		fmt.Fprintf(os.Stdout, "temp_code: %s\n", s.TempCode)
		fmt.Fprintf(os.Stdout, "\n")
	})
	cc.OnStop(func(s *cowtransfer.UploadSession) {
		fmt.Fprintf(os.Stdout, "event: session_stop\n")
		fmt.Fprintf(os.Stdout, "upload_token: %s\n", s.UploadToken)
		fmt.Fprintf(os.Stdout, "transfer_guid: %s\n", s.TransferGUID)
		fmt.Fprintf(os.Stdout, "file_guid: %s\n", s.FileGUID)
		fmt.Fprintf(os.Stdout, "url: %s\n", s.UniqueURL)
		fmt.Fprintf(os.Stdout, "prefix: %s\n", s.Prefix)
		fmt.Fprintf(os.Stdout, "qrcode: %s\n", s.QRCode)
		fmt.Fprintf(os.Stdout, "temp_code: %s\n", s.TempCode)
		fmt.Fprintf(os.Stdout, "\n")
	})
	cc.OnFileTransfer(func(fi *cowtransfer.FileTransfer) {
		fmt.Fprintf(os.Stdout, "event: file_transfer\n")
		fmt.Fprintf(os.Stdout, "path: %s\n", fi.Path)
		fmt.Fprintf(os.Stdout, "state: %s\n", fi.State.String())
		fmt.Fprintf(os.Stdout, "total_size: %d\n", fi.Size)
		fmt.Fprintf(os.Stdout, "done_size: %d\n", fi.DoneSize)
		fmt.Fprintf(os.Stdout, "total_blocks: %d\n", fi.Blocks)
		fmt.Fprintf(os.Stdout, "done_blocks: %d\n", fi.DoneBlocks)
		fmt.Fprintf(os.Stdout, "block: %d\n", fi.BlockNumber)
		fmt.Fprintf(os.Stdout, "block_size: %d\n", fi.BlockSize)
		if fi.Error != nil {
			fmt.Fprintf(os.Stdout, "retry: %d\n", fi.Retry)
			fmt.Fprintf(os.Stdout, "retries_left: %d\n", fi.RetriesLeft)
			fmt.Fprintf(os.Stdout, "error: %s\n", fi.Error.Error())
		}
		fmt.Fprintf(os.Stdout, "\n")
	})


	dlURL, err := cc.Upload(files...)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stdout, "link: %s\n", dlURL)
	return nil
}

func listRemoteFiles(url string) error {
	cc := cowtransfer.NewClient()
	
	files, err := cc.Files(url)
	if err != nil {
		return fmt.Errorf("cannot resolve %s: %v\n", url, err)
	}

	for i, v := range files {
		fmt.Fprintf(os.Stdout, "index: %d\n", i)
		fmt.Fprintf(os.Stdout, "filename: %s\n", v.FileName)
		fmt.Fprintf(os.Stdout, "size: %d\n", v.Size)
		fmt.Fprintf(os.Stdout, "url: %s\n", v.URL)
		if v.Error != nil {
			fmt.Fprintf(os.Stdout, "error: %s\n", v.Error.Error())
		}
		fmt.Fprintf(os.Stdout, "\n")
	}
	return nil
}