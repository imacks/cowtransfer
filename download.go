package cowtransfer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"
	"strconv"
)

const (
	downloadDetailsURL = "https://cowtransfer.com/transfer/transferdetail?url=%s&treceive=undefined&passcode=%s"
	downloadConfigURL  = "https://cowtransfer.com/transfer/download?guid=%s"
)

var regex = regexp.MustCompile("[0-9a-f]{14}")

// CowFileInfo represents information about a file
type CowFileInfo struct {
	FileName string `json:"fileName"`
	Size     int64  `json:"size"`
	URL      string `json:"url"`
	Error    error  `json:"error"`
}

type downloadDetailsResponse struct {
	GUID         string                 `json:"guid"`
	DownloadName string                 `json:"downloadName"`
	Deleted      bool                   `json:"deleted"`
	Uploaded     bool                   `json:"uploaded"`
	Details      []downloadDetailsBlock `json:"transferFileDtos"`
}

type downloadDetailsBlock struct {
	GUID     string `json:"guid"`
	FileName string `json:"fileName"`
	Size     string `json:"size"`
}

type downloadConfigResponse struct {
	Link string `json:"link"`
}

// Files return information on all files in a download link
func (cc *CowClient) Files(link string) ([]CowFileInfo, error) {
	fileID := regex.FindString(link)
	if fileID == "" {
		return nil, fmt.Errorf("unknown download code format")
	}

	detailsURL := fmt.Sprintf(downloadDetailsURL, fileID, cc.Password)
	response, err := http.Get(detailsURL)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", detailsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", fmt.Sprintf("https://cowtransfer.com/s/%s", fileID))
	req.Header.Set("Cookie", fmt.Sprintf("cf-cs-k-20181214=%d;", time.Now().UnixNano()))
	response, err = http.DefaultClient.Do(req)
	if err != nil { 
		return nil, err
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	_ = response.Body.Close()

	details := new(downloadDetailsResponse)
	if err := json.Unmarshal(body, details); err != nil {
		fmt.Printf("fsd %s\n", string(body))
		return nil, err
	}

	if details.GUID == "" {
		return nil, fmt.Errorf("link invalid")
	} else if details.Deleted {
		return nil, fmt.Errorf("link deleted")
	} else if !details.Uploaded {
		return nil, fmt.Errorf("link not finish upload yet")
	}

	result := []CowFileInfo{}
	for _, item := range details.Details {
		cowfi := cc.getItemDownloadURL(item)
		result = append(result, cowfi)
	}

	return result, nil
}

func (cc *CowClient) getItemDownloadURL(item downloadDetailsBlock) CowFileInfo {
	result := CowFileInfo{
		FileName: item.FileName,
	}

	configURL := fmt.Sprintf(downloadConfigURL, item.GUID)
	req, err := http.NewRequest("POST", configURL, nil)
	if err != nil {
		result.Error = err
		return result
	}

	response, err := http.DefaultClient.Do(addHeaders(req, cc.UserAgent))
	if err != nil {
		result.Error = err
		return result
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		result.Error = err
		return result
	}
	_ = response.Body.Close()

	config := new(downloadConfigResponse)
	if err := json.Unmarshal(body, config); err != nil {
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