package cowtransfer

import (
	"encoding/json"
	"time"
)

const (
	defaultUA          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.102 Safari/537.36 Edg/85.0.564.51"
	defaultBlockSize   = 4194304 // 4096kb
)

// UploadSession is a file upload session. Multiple files can be uploaded in a 
// single session.
type UploadSession struct {
	UploadToken  string `json:"uptoken"`
	TransferGUID string `json:"transferguid"`
	FileGUID     string `json:"fileguid"`
	UniqueURL    string `json:"uniqueurl"`
	Prefix       string `json:"prefix"`
	QRCode       string `json:"qrcode"`
	TempCode     string `json:"temp_download_code"`
}

// File represents a file transfer operation.
type FileTransfer struct {
	// Path to file on local filesystem.
	Path string              `json:"path"`
	// Size of file.
	Size int64               `json:"size"`
	// State is current transfer state.
	State TransferState      `json:"state"`
	// Blocks is total number of blocks.
	Blocks int64             `json:"blocks"`
	// DoneBlocks is the number of processed blocks.
	DoneBlocks int64         `json:"done_blocks"`
	// DoneSize is the total size of blocks processed.
	DoneSize int64	         `json:"done_size"`
	// BlockNumber is block index (1-based) currently being processed.
	BlockNumber int64        `json:"block_num"`
	// BlockSize is the size of the block currently being processed.
	BlockSize int            `json:"block_size"`
	// Retry is the current retry number.
	Retry int                `json:"retry"`
	// RetriesLeft is the number of retries remaining before giving up.
	RetriesLeft int          `json:"retries_left"`
	// Error is the error encountered in the last (retry) operation.
	Error error              `json:"error"`
}

func (f *FileTransfer) MarshalJSON() ([]byte, error) {
	type fileAlias FileTransfer
	return json.Marshal(&struct {
		State    string  `json:"state"`
		*fileAlias
	}{
		State:       f.State.String(),
		fileAlias:   (*fileAlias)(f),
	})
}

// TransferState represents the state of a file transfer operation.
type TransferState int
const (
	InitTransfer TransferState = iota
	FinishTransfer
	RequestUpload
	ConfirmUpload
	Uploading
	Downloading
	DoBlock
	DoneBlock
	RetryBlock
)

func (ts TransferState) String() string {
	switch ts {
	case InitTransfer:
		return "InitTransfer"
	case FinishTransfer:
		return "FinishTransfer"
	case RequestUpload:
		return "RequestUpload"
	case ConfirmUpload:
		return "ConfirmUpload"
	case Uploading:
		return "Uploading"
	case Downloading:
		return "Downloading"
	case DoBlock:
		return "DoBlock"
	case DoneBlock:
		return "DoneBlock"
	case RetryBlock:
		return "RetryBlock"
	default:
		return "undefined"
	}
}

// FileTransferFunc is a file transfer progress hook.
type FileTransferFunc func(ft *FileTransfer)
// SessionOpenCloseFunc is a session creation and close event hook.
type SessionOpenCloseFunc func(s *UploadSession)
// PushBlockErrorHandler is a handler for block upload failure.
type PushBlockErrorHandler func(ft *FileTransfer) error

// CowClient is a client for CowTransfer.cn
type CowClient struct {
	// Timeout is the HTTP client timeout duration. Defaults to 10 seconds.
	Timeout time.Duration
	// MaxRetry is the maximum number of retries before giving up. Defaults to 
	// 3 tries.
	MaxRetry int
	// UserAgent overrides the default HTTP client useragent.
	UserAgent string
	// VerifyHash will use MD5 checksum to verify each block.
	VerifyHash bool
	// Password is an optional password that is used to protect content from 
	// downloads.
	Password string
	// Token overrides the default cookie.
	Token string
	// BlockSize is the size of each file part to download or upload. Defaults 
	// to 4096kb.
	BlockSize int
	// MaxPushBlocks is the maximum number of file parts to upload 
	// concurrently. This will be averaged amongst MaxPushFiles.
	MaxPushBlocks int
	// APIURL overrides the default Cowtransfer API endpoint.
	APIURL string
	// OSSURL overrides the default Qiniu OSS API endpoint.
	OSSURL string
	// progress hooks
	transferProgressHook FileTransferFunc
	openSessionHook SessionOpenCloseFunc
	closeSessionHook SessionOpenCloseFunc
}

// NewClient creates a new CowClient instance with default values.
func NewClient() *CowClient {
	return &CowClient{
		UserAgent: defaultUA,
		BlockSize: defaultBlockSize,
		MaxPushBlocks: 1,
		Timeout: 10*time.Second,
		Token: "",
		Password: "",
		VerifyHash: true,
		MaxRetry: 3,
		APIURL: defaultAPIURL,
		OSSURL: defaultOSSURL,
	}
}

// OnStart is a progress hook for session start.
func (cc *CowClient) OnStart(hook SessionOpenCloseFunc) {
	cc.openSessionHook = hook
}

// OnStart is a progress hook for session stop.
func (cc *CowClient) OnStop(hook SessionOpenCloseFunc) {
	cc.closeSessionHook = hook
}

// OnFileTransfer is a progress hook for file transfer progress.
func (cc *CowClient) OnFileTransfer(hook FileTransferFunc) {
	cc.transferProgressHook = hook
}
