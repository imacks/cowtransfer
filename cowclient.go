package cowtransfer

import (
	"time"
	"os"
	"fmt"
)

const (
	defaultUA          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.102 Safari/537.36 Edg/85.0.564.51"
	defaultBlockSize   = 4194304
)

// ProgressHook is a hook for upload and download progress
type ProgressHook struct {
	Start func(fi os.FileInfo)
	Finish func(fi os.FileInfo)
	Seal func(fi os.FileInfo)
	Block func(item int64, current, max int)
	Retry func(op string, count int, err error)
	RetryBlock func(op string, item int64, block int, err error)
}

// CowClient is a client for CowTransfer.cn
type CowClient struct {
	Timeout time.Duration
	MaxRetry int
	UserAgent string
	Progress ProgressHook
	VerifyHash bool
	Password string
	UploadParallel int
	DownloadParallel int
	Token string
	BlockSize int
}

// NewCowClient creates a CowClient using default parameters
func NewCowClient() *CowClient {
	progHook := ProgressHook{
		Start: func(fi os.FileInfo) {
			fmt.Printf("[start] file %s | size %d\n", fi.Name(), fi.Size())
		},
		Finish: func(fi os.FileInfo) {
			fmt.Printf("[finish] file %s | size %d\n", fi.Name(), fi.Size())
		},
		Seal: func(fi os.FileInfo) {
			fmt.Printf("[seal] file %s | size %d\n", fi.Name(), fi.Size())
		},
		Block: func(item int64, current, max int) {
			fmt.Printf("[block] item %d | current %d | len %d\n", item, current, max)
		},
		RetryBlock: func(op string, item int64, block int, err error) {
			fmt.Printf("[blkerror] %s | item %d | block %d | %v\n", op, item, block, err)
		},
		Retry: func(op string, count int, err error) {
			fmt.Printf("[error] retry %s | attempt %d | %v\n", op, count, err)
		},
	}

	return &CowClient{
		UserAgent: defaultUA,
		BlockSize: defaultBlockSize,
		UploadParallel: 4,
		DownloadParallel: 4,
		Timeout: time.Duration(10) * time.Second,
		Token: "",
		Password: "",
		VerifyHash: true,
		MaxRetry: 3,
		Progress: progHook,
	}
}