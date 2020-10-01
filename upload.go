package cowtransfer

import (
	"bytes"
	"fmt"
	"time"
	"os"
	"math"
	"strings"
	"sync"
	"strconv"
	"path/filepath"
	"io"
	"io/ioutil"
	"hash/crc32"
	"encoding/json"
	"encoding/base64"
	"mime/multipart"
	"net/http"

	cmap "github.com/orcaman/concurrent-map"
)

const (
	refererURL            = "https://cowtransfer.com/"
	prepareSendURL        = "https://cowtransfer.com/transfer/preparesend"
	setPasswordURL        = "https://cowtransfer.com/transfer/v2/bindpasscode"
	beforeUploadURL       = "https://cowtransfer.com/transfer/beforeupload"
	uploadInitURL         = "https://upload.qiniup.com/mkblk/%d"
	uploadURL             = "https://upload.qiniup.com/bput/%s/%d"
	uploadFinishURL       = "https://cowtransfer.com/transfer/uploaded"
	uploadCompleteURL     = "https://cowtransfer.com/transfer/complete"
	uploadMergeFileURL    = "https://upload.qiniup.com/mkfile/%s/key/%s/fname/%s"
)

type uploadPart struct {
	content []byte
	count   int64
}

type uploadResponse struct {
	Ticket string `json:"ctx"`
	Hash   int    `json:"crc32"`
}

type beforeSendResponse struct {
	FileGUID string `json:"fileGuid"`
}

type finishResponse struct {
	TempDownloadCode string `json:"tempDownloadCode"`
	Status           bool   `json:"complete"`
}

type uploadResult struct {
	Hash string `json:"hash"`
	Key  string `json:"key"`
}

type prepareSendResponse struct {
	UploadToken  string `json:"uptoken"`
	TransferGUID string `json:"transferguid"`
	FileGUID     string `json:"fileguid"`
	UniqueURL    string `json:"uniqueurl"`
	Prefix       string `json:"prefix"`
	QRCode       string `json:"qrcode"`
	Error        bool   `json:"error"`
	ErrorMessage string `json:"error_message"`
}

// Upload uploads a list of files to CowTransfer
func (cc *CowClient) Upload(files []string) (string, error) {
	totalSize := int64(0)

	filePaths := []string{}
	for _, v := range files {
		if !isExist(v) {
			return "", fmt.Errorf("file or directory not found: %s", v)
		}

		err := filepath.Walk(v, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if fi.IsDir() {
				return nil
			}
			totalSize += fi.Size()
			filePaths = append(filePaths, path)
			return nil
		})
		if err != nil {
			return "", err
		}
	}

	sendConf, err := cc.getSendConfig(totalSize)
	if err != nil {
		return "", err
	}

	for _, v := range filePaths {
		err = cc.uploadSingleFile(v, sendConf)
		if err != nil {
			return "", err
		}
	}

	_, err = cc.completeUpload(sendConf)
	if err != nil {
		return "", err
	}

	return sendConf.UniqueURL, nil
}

func (cc *CowClient) uploadSingleFile(file string, sendConf *prepareSendResponse) error {
	fi, err := getFileInfo(file)
	if err != nil {
		return err
	}

	uploadConf, err := cc.getUploadConfig(fi, sendConf)
	if err != nil {
		return err
	}

	f, err := os.Open(file)
	if err != nil {
		return err
	}

	cc.Progress.Start(fi)

	wg := new(sync.WaitGroup)
	ch := make(chan *uploadPart)
	hashMap := cmap.New()
	for i := 0; i < cc.UploadParallel; i++ {
		go cc.uploadBlock(&ch, wg, uploadConf.UploadToken, &hashMap)
	}

	part := int64(0)
	for {
		part++
		buf := make([]byte, cc.BlockSize)
		nr, err := f.Read(buf)
		if nr <= 0 || err != nil {
			break
		}
		if nr > 0 {
			wg.Add(1)
			ch <- &uploadPart{
				content: buf[:nr],
				count:   part,
			}
		}
	}

	wg.Wait()
	close(ch)
	cc.Progress.Finish(fi)

	err = cc.finishUpload(uploadConf, fi, &hashMap, part)
	if err != nil {
		return err
	}
	cc.Progress.Seal(fi)

	return nil
}

func (cc *CowClient) uploadBlock(ch *chan *uploadPart, wg *sync.WaitGroup, token string, hashMap *cmap.ConcurrentMap) {
	for item := range *ch {
		postURL := fmt.Sprintf(uploadInitURL, len(item.content))

		// makeBlock
		body, err := cc.newRequest(postURL, nil, token, 0)
		if err != nil {
			cc.Progress.RetryBlock("mkblk", item.count, -1, err)
			*ch <- item
			continue
		}

		var rBody uploadResponse
		if err := json.Unmarshal(body, &rBody); err != nil {
			cc.Progress.RetryBlock("unmarshal", item.count, -1, err)
			*ch <- item
			continue
		}

		// blockPut
		failFlag := false
		blockCount := int(math.Ceil(float64(len(item.content)) / float64(cc.BlockSize)))
		cc.Progress.Block(item.count, 0, blockCount)

		ticket := rBody.Ticket
		for i := 0; i < blockCount; i++ {
			start := i * cc.BlockSize
			end := (i + 1) * cc.BlockSize
			var buf []byte
			if end > len(item.content) {
				buf = item.content[start:]
			} else {
				buf = item.content[start:end]
			}
			postURL = fmt.Sprintf(uploadURL, ticket, start)
			ticket, err = cc.blockPut(postURL, buf, token, 0)
			if err != nil {
				cc.Progress.RetryBlock("blockput", item.count, i, err)
				failFlag = true
				break
			}
			cc.Progress.Block(item.count, i, blockCount)
		}
		if failFlag {
			*ch <- item
			continue
		}

		cc.Progress.Block(item.count, blockCount, blockCount)
		hashMap.Set(strconv.FormatInt(item.count, 10), ticket)
		wg.Done()
	}
}

func (cc *CowClient) newMultipartRequest(url string, params map[string]string, retryCount int) ([]byte, error) {
	client := http.Client{Timeout: cc.Timeout}
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	for key, val := range params {
		_ = writer.WriteField(key, val)
	}
	_ = writer.Close()

	req, err := http.NewRequest("POST", url, buf)
	if err != nil {
		cc.Progress.Retry("multipart:create", retryCount, err)
		if retryCount > cc.MaxRetry {
			return nil, err
		}
		return cc.newMultipartRequest(url, params, retryCount+1)
	}
	req.Header.Set("content-type", fmt.Sprintf("multipart/form-data;boundary=%s", writer.Boundary()))
	req.Header.Set("referer", refererURL)
	req.Header.Set("cookie", cc.Token)

	resp, err := client.Do(addHeaders(req, cc.UserAgent))
	if err != nil {
		cc.Progress.Retry("multipart:post", retryCount, err)
		if retryCount > cc.MaxRetry {
			return nil, err
		}
		return cc.newMultipartRequest(url, params, retryCount+1)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		cc.Progress.Retry("multipart:read-response", retryCount, err)
		if retryCount > cc.MaxRetry {
			return nil, err
		}
		return cc.newMultipartRequest(url, params, retryCount+1)
	}
	_ = resp.Body.Close()

	if s := resp.Header.Values("Set-Cookie"); len(s) != 0 && cc.Token == "" {
		for _, v := range s {
			ck := strings.Split(v, ";")
			cc.Token += ck[0] + ";"
		}
	}

	return body, nil
}

func (cc *CowClient) newRequest(url string, postBody io.Reader, upToken string, retryCount int) ([]byte, error) {
	client := http.Client{Timeout: cc.Timeout}

	req, err := http.NewRequest("POST", url, postBody)
	if err != nil {
		cc.Progress.Retry("newrequest:create", retryCount, err)
		if retryCount > cc.MaxRetry {
			return nil, err
		}
		return cc.newRequest(url, postBody, upToken, retryCount+1)
	}
	req.Header.Set("referer", refererURL)
	req.Header.Set("Authorization", "UpToken "+upToken)

	resp, err := client.Do(req)
	if err != nil {
		cc.Progress.Retry("newrequest:post", retryCount, err)
		if retryCount > cc.MaxRetry {
			return nil, err
		}
		return cc.newRequest(url, postBody, upToken, retryCount+1)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		cc.Progress.Retry("newrequest:read-response", retryCount, err)
		if retryCount > cc.MaxRetry {
			return nil, err
		}
		return cc.newRequest(url, postBody, upToken, retryCount+1)
	}

	_ = resp.Body.Close()
	return body, nil
}

func (cc *CowClient) blockPut(url string, buf []byte, token string, retryCount int) (string, error) {
	data := new(bytes.Buffer)
	data.Write(buf)
	body, err := cc.newRequest(url, data, token, 0)
	if err != nil {
		cc.Progress.Retry("blockput:request", retryCount, err)
		if retryCount > cc.MaxRetry {
			return "", err
		}
		return cc.blockPut(url, buf, token, retryCount+1)
	}

	var rBody uploadResponse
	if err := json.Unmarshal(body, &rBody); err != nil {
		cc.Progress.Retry("blockput:marshal", retryCount, err)
		if retryCount > cc.MaxRetry {
			return "", err
		}
		return cc.blockPut(url, buf, token, retryCount+1)
	}

	if cc.VerifyHash {
		if hashBlock(buf) != rBody.Hash {
			cc.Progress.Retry("blockput:hash", retryCount, err)
			if retryCount > cc.MaxRetry {
				return "", err
			}
			return cc.blockPut(url, buf, token, retryCount+1)
		}
	}

	return rBody.Ticket, nil
}

func (cc *CowClient) getUploadConfig(fi os.FileInfo, r *prepareSendResponse) (*prepareSendResponse, error) {
	data := map[string]string{
		"fileId":        "",
		"type":          "",
		"fileName":      fi.Name(),
		"originalName":  fi.Name(),
		"fileSize":      strconv.FormatInt(fi.Size(), 10),
		"transferGuid":  r.TransferGUID,
		"storagePrefix": r.Prefix,
	}

	response, err := cc.newMultipartRequest(beforeUploadURL, data, 0)
	if err != nil {
		return nil, err
	}

	var beforeSend *beforeSendResponse
	if err = json.Unmarshal(response, &beforeSend); err != nil {
		return nil, err
	}

	r.FileGUID = beforeSend.FileGUID
	return r, nil
}

func (cc *CowClient) getSendConfig(totalSize int64) (*prepareSendResponse, error) {
	data := map[string]string{
		"totalSize": strconv.FormatInt(totalSize, 10),
	}

	body, err := cc.newMultipartRequest(prepareSendURL, data, 0)
	if err != nil {
		return nil, err
	}

	r := new(prepareSendResponse)
	err = json.Unmarshal(body, &r)
	if err != nil {
		return nil, err
	}

	if r.Error {
		return nil, fmt.Errorf(r.ErrorMessage)
	}

	if cc.Password != "" {
		// set password
		data := map[string]string{
			"transferguid": r.TransferGUID,
			"passcode":     cc.Password,
		}
		body, err = cc.newMultipartRequest(setPasswordURL, data, 0)
		if err != nil {
			return nil, err
		}
		if string(body) != "true" {
			return nil, fmt.Errorf("set password unsuccessful")
		}
	}

	return r, nil
}

func (cc *CowClient) completeUpload(r *prepareSendResponse) (string, error) {
	data := map[string]string{
		"transferGuid": r.TransferGUID, 
		"fileId": "",
	}

	body, err := cc.newMultipartRequest(uploadCompleteURL, data, 0)
	if err != nil {
		return "", err
	}

	var rBody finishResponse
	if err := json.Unmarshal(body, &rBody); err != nil {
		return "", fmt.Errorf("read finish response failed: %s", err)
	}

	if !rBody.Status {
		return "", fmt.Errorf("finish upload failed: complete is not true")
	}

	return rBody.TempDownloadCode, nil
}

func (cc *CowClient) finishUpload(r *prepareSendResponse, fi os.FileInfo, hashMap *cmap.ConcurrentMap, limit int64) error {
	filename := urlSafeEncode(fi.Name())
	var fileLocate string
	fileLocate = urlSafeEncode(fmt.Sprintf("%s/%s/%s", r.Prefix, r.TransferGUID, fi.Name()))

	mergeFileURL := fmt.Sprintf(uploadMergeFileURL, strconv.FormatInt(fi.Size(), 10), fileLocate, filename)
	postBody := ""
	for i := int64(1); i <= limit; i++ {
		item, alimasu := hashMap.Get(strconv.FormatInt(i, 10))
		if alimasu {
			postBody += item.(string) + ","
		}
	}
	if strings.HasSuffix(postBody, ",") {
		postBody = postBody[:len(postBody)-1]
	}

	reader := bytes.NewReader([]byte(postBody))
	response, err := cc.newRequest(mergeFileURL, reader, r.UploadToken, 0)
	if err != nil {
		return err
	}

	// read returns
	var mergeResponse *uploadResult
	if err = json.Unmarshal(response, &mergeResponse); err != nil {
		return err
	}

	data := map[string]string{
		"transferGuid": r.TransferGUID,
		"fileGuid": r.FileGUID,
		"hash": mergeResponse.Hash,
	}
	body, err := cc.newMultipartRequest(uploadFinishURL, data, 0)
	if err != nil {
		return err
	}
	if string(body) != "true" {
		return fmt.Errorf("finish upload failed: status != true")
	}

	return nil
}

func addHeaders(req *http.Request, ua string) *http.Request {
	if ua == "" {
		ua = defaultUA
	}

	req.Header.Set("Referer", refererURL)
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Origin", refererURL)
	req.Header.Set("Cookie", fmt.Sprintf("%scf-cs-k-20181214=%d;", req.Header.Get("Cookie"), time.Now().UnixNano()))
	return req
}

func getFileInfo(path string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func urlSafeEncode(enc string) string {
	r := base64.StdEncoding.EncodeToString([]byte(enc))
	r = strings.ReplaceAll(r, "+", "-")
	r = strings.ReplaceAll(r, "/", "_")
	return r
}

func hashBlock(buf []byte) int {
	return int(crc32.ChecksumIEEE(buf))
}