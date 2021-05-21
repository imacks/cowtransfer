package cowtransfer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

const (
	downloadDetailsURL = "%s/transfer/transferdetail?url=%s&treceive=undefined&passcode=%s"
	downloadConfigURL  = "%s/transfer/download?guid=%s"
	downloadFilesURL   = "%s/transfer/files?page=%d&guid=%s"
)

// FileInfo represents information about a remote file.
type FileInfo struct {
	FileName string `json:"fileName"`
	Size     int64  `json:"size"`
	URL      string `json:"url"`
	Error    error  `json:"error"`
}

// downloadDetailsResponse is expected response from downloadDetailsURL API.
type downloadDetailsResponse struct {
	GUID         string                 `json:"guid"`
	DownloadName string                 `json:"downloadName"`
	Deleted      bool                   `json:"deleted"`
	Uploaded     bool                   `json:"uploaded"`
}

type downloadFilesResponse struct {
	Details []downloadDetailsBlock `json:"transferFileDtos"`
	Pages   int64                  `json:"totalPages"`
}

type downloadDetailsBlock struct {
	GUID     string `json:"guid"`
	FileName string `json:"fileName"`
	Size     string `json:"size"`
}

// downloadConfigResponse is expected response from downloadConfigURL API.
type downloadConfigResponse struct {
	Link string `json:"link"`
}

var fileIDRegex = regexp.MustCompile("[0-9a-f]{14}")

// Files return information on all files in a download link.
func (cc *CowClient) Files(url string) ([]FileInfo, error) {
	fileID := fileIDRegex.FindString(url)
	if fileID == "" {
		return nil, ErrDownloadURL
	}

	detailsURL := fmt.Sprintf(downloadDetailsURL, cc.APIURL, fileID, cc.Password)
	responseBytes, err := cc.newFileDownloadRequest(detailsURL, fileID)
	if err != nil {
		return nil, err
	}

	allFiles := new(downloadDetailsResponse)
	if err := json.Unmarshal(responseBytes, allFiles); err != nil {
		return nil, err
	}

	if allFiles.GUID == "" {
		return nil, ErrDownloadNotFound
	} else if allFiles.Deleted {
		return nil, ErrDownloadDeleted
	} else if !allFiles.Uploaded {
		return nil, ErrUploadInProgress
	}

	pageInfo, err := cc.getFilesByPage(0, allFiles.GUID, fileID)
	if err != nil {
		return nil, err
	}

	if pageInfo.Pages > 1 {
		for i := 0; i < int(pageInfo.Pages); i++ {
			more, err := cc.getFilesByPage(i, allFiles.GUID, fileID)
			if err != nil {
				return nil, err
			}
			pageInfo.Details = append(pageInfo.Details, more.Details...)
		}
	}

	result := []FileInfo{}
	for _, item := range pageInfo.Details {
		cowfi := cc.getFileURL(&item)
		result = append(result, cowfi)
	}

	return result, nil
}

func (cc *CowClient) getFilesByPage(page int, guid, fileID string) (*downloadFilesResponse, error) {
	responseBytes, err := cc.newFileDownloadRequest(fmt.Sprintf(downloadFilesURL, cc.APIURL, page, guid), fileID)
	if err != nil {
		return nil, err
	}

	pageInfo := new(downloadFilesResponse)
	if err := json.Unmarshal(responseBytes, pageInfo); err != nil {
		return nil, err
	}

	return pageInfo, nil
}

func (cc *CowClient) getFileURL(item *downloadDetailsBlock) FileInfo {
	result := FileInfo{
		FileName: item.FileName,
	}

	configURL := fmt.Sprintf(downloadConfigURL, cc.APIURL, item.GUID)
	req, err := http.NewRequest("POST", configURL, nil)
	if err != nil {
		result.Error = err
		return result
	}

	client := http.Client{Timeout: cc.Timeout}
	response, err := client.Do(cc.addHeaders(req))
	if err != nil {
		result.Error = err
		return result
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		result.Error = err
		return result
	}
	_ = response.Body.Close()

	config := new(downloadConfigResponse)
	if err := json.Unmarshal(bodyBytes, config); err != nil {
		result.Error = err
		return result
	}
	result.URL = config.Link

	numSize, err := strconv.ParseFloat(item.Size, 10)
	if err != nil {
		result.Error = err
		result.Size = 0
		return result
	}

	result.Size = int64(numSize * 1024)
	return result
}

// newFileDownloadRequest is a general wrapper for download related API calls.
func (cc *CowClient) newFileDownloadRequest(url, fileID string) ([]byte, error) {
	client := http.Client{Timeout: cc.Timeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", fmt.Sprintf("%s/s/%s", cc.APIURL, fileID))
	req.Header.Set("Cookie", fmt.Sprintf(cc.Token, "", time.Now().UnixNano()))

	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}	
	_ = response.Body.Close()

	return bodyBytes, nil
}
