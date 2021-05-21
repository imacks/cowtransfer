package cowtransfer

import "errors"

var (
	ErrInvalidResponse = errors.New("invalid response from endpoint")
	ErrBlockChecksum = errors.New("block has invalid checksum")
	ErrDownloadURL = errors.New("unsupported download URL")
	ErrDownloadNotFound = errors.New("download not found")
	ErrDownloadDeleted = errors.New("download is already deleted")
	ErrUploadInProgress = errors.New("upload in progress")
)