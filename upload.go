package cowtransfer

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultCookieFormat   = "%scf-cs-k-20181214=%d;"
	defaultAPIURL         = "https://cowtransfer.com"
	defaultOSSURL         = "https://upload-fog-cn-east-1.qiniup.com"
	// restful apis for cowtransfer and qiniu oss
	createUploadSessionURL = "%s/transfer/preparesend"
	setPullPasswordURL     = "%s/transfer/v2/bindpasscode"
	finishUploadSessionURL = "%s/transfer/complete"
	uploadFileURL          = "%s/transfer/beforeupload"
	finishUploadFileURL    = "%s/transfer/uploaded"
	ossInitPushURL         = "%s/buckets/cowtransfer-yz/objects/%s/uploads"
	ossPushBlockURL        = "%s/buckets/cowtransfer-yz/objects/%s/uploads/%s/%d"
	ossFinishPushURL       = "%s/buckets/cowtransfer-yz/objects/%s/uploads/%s"
)

// uploadSessionResponse is expected response from createUploadSessionURL API.
type uploadSessionResponse struct {
	UploadToken  string `json:"uptoken"`
	TransferGUID string `json:"transferguid"`
	FileGUID     string `json:"fileguid"`
	UniqueURL    string `json:"uniqueurl"`
	Prefix       string `json:"prefix"`
	QRCode       string `json:"qrcode"`
	Error        bool   `json:"error"`
	ErrorMessage string `json:"error_message"`
}

// uploadSessionFinish is expected response from finishUploadSessionURL API. 
type uploadSessionFinishResponse struct {
	TempDownloadCode string `json:"tempDownloadCode"`
	Status           bool   `json:"complete"`
}

// uploadFileResponse is expected response from uploadFileURL API.
type uploadFileResponse struct {
	FileGuid string `json:"fileGuid"`
}

// ossInitUploadResponse is expected response from ossInitPushURL API.
type ossInitUploadResponse struct {
	Token        string
	TransferGUID string
	FileGUID     string
	EncodeID     string
	Exp          int64  `json:"expireAt"`
	ID           string `json:"uploadId"`
}

// ossMergeBlocksRequest is the request body to send to ossFinishPushURL API.
type ossMergeBlocksRequest struct {
	Parts    []fileBlockSlek `json:"parts"`
	FName    string          `json:"fname"`
	Mimetype string          `json:"mimeType"`
	Metadata map[string]string
	Vars     map[string]string
}

type fileBlockSlek struct {
	ETag string `json:"etag"`
	Part int64  `json:"partNumber"`
}

// ossMergeBlocksResponse is the expected response from ossFinishPushURL API.
type ossMergeBlocksResponse struct {
	Hash string `json:"hash"`
	Key  string `json:"key"`
}

// ossPushBlockResponse is the expected response from ossPushBlockURL API.
type ossPushBlockResponse struct {
	Etag string `json:"etag"`
	MD5  string `json:"md5"`
}

type fileBlockUpload struct {
	filePath    string
	fileSize    int64
	content     []byte
	count       int64
	totalBlocks int64
}

// Upload a list of files to CowTransfer. Returns the unique download URL if 
// all uploads are successful.
func (cc *CowClient) Upload(files ...string) (string, error) {
	filePaths, totalSize, err := listFilesInPath(files...)
	if err != nil {
		return "", err
	}

	session, err := cc.newUploadSession(totalSize)
	if err != nil {
		return "", err
	}
	if cc.openSessionHook != nil {
		cc.openSessionHook(&UploadSession{
			UploadToken: session.UploadToken,
			TransferGUID: session.TransferGUID,
			FileGUID: session.FileGUID,
			UniqueURL: session.UniqueURL,
			Prefix: session.Prefix,
			QRCode: session.QRCode,
			TempCode: "",
		})
	}

	for _, v := range filePaths {
		if cc.MaxPushBlocks < 2 {
			err = cc.uploadFileBlocksSerial(v, session)
		} else {
			err = cc.uploadFileBlocksParallel(v, session)
		}
		if err != nil {
			return "", err
		}
	}

	tmpCode, err := cc.finishUploadSession(session)
	if err != nil {
		return "", err
	}
	if cc.closeSessionHook != nil {
		cc.closeSessionHook(&UploadSession{
			UploadToken: session.UploadToken,
			TransferGUID: session.TransferGUID,
			FileGUID: session.FileGUID,
			UniqueURL: session.UniqueURL,
			Prefix: session.Prefix,
			QRCode: session.QRCode,
			TempCode: tmpCode,
		})
	}

	return session.UniqueURL, nil
}

func (cc *CowClient) newUploadSession(totalSize int64) (*uploadSessionResponse, error) {
	data := map[string]string{
		"totalSize": strconv.FormatInt(totalSize, 10),
	}
	body, err := cc.newMultipartFormRequest(fmt.Sprintf(createUploadSessionURL, cc.APIURL), data)
	if err != nil {
		return nil, err
	}

	session := new(uploadSessionResponse)
	err = json.Unmarshal(body, &session)
	if err != nil {
		return nil, err
	}
	if session.Error {
		return nil, fmt.Errorf(session.ErrorMessage)
	}

	if cc.Password != "" {
		// set password
		data := map[string]string{
			"transferguid": session.TransferGUID,
			"passcode":     cc.Password,
		}
		body, err = cc.newMultipartFormRequest(setPullPasswordURL, data)
		if err != nil {
			return nil, err
		}
		if string(body) != "true" {
			return nil, ErrInvalidResponse
		}
	}
	return session, nil
}

func (cc *CowClient) finishUploadSession(s *uploadSessionResponse) (string, error) {
	data := map[string]string{
		"transferGuid": s.TransferGUID,
		"fileId": "",
	}

	bodyBytes, err := cc.newMultipartFormRequest(fmt.Sprintf(finishUploadSessionURL, cc.APIURL), data)
	if err != nil {
		return "", err
	}

	var response uploadSessionFinishResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return "", fmt.Errorf("cannot marshal response: %v", err)
	}
	if !response.Status {
		return response.TempDownloadCode, fmt.Errorf("complete status is false")
	}

	return response.TempDownloadCode, nil
}

// uploadFileBlocksSerial uploads a file one block at a time.
func (cc *CowClient) uploadFileBlocksSerial(filePath string, session *uploadSessionResponse) error {
	fi, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot read file %s: %v", filePath, err)
	}
	// estimate the total number of blocks to upload
	fileSize := fi.Size()
	totalBlocks := blocksInFile(fileSize, cc.BlockSize)

	if cc.transferProgressHook != nil {
		cc.transferProgressHook(&FileTransfer{
			Path: filePath,
			State: InitTransfer,
			Size: fileSize,
			Blocks: totalBlocks,
		})
	}

	uploadJob, err := cc.newFileUpload(fi, session)
	if err != nil {
		return err
	}

	uploadFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file %s: %v", filePath, err)
	}

	hashmap := map[int64]string{}
	parts := int64(0)
	for {
		buffer := make([]byte, cc.BlockSize)
		nr, err := uploadFile.Read(buffer)
		// #todo handle err
		if nr <= 0 || err != nil {
			break
		}
		parts++
		if nr > 0 {
			putURL := fmt.Sprintf(ossPushBlockURL, cc.OSSURL, uploadJob.EncodeID, uploadJob.ID, parts)

			if cc.transferProgressHook != nil {
				cc.transferProgressHook(&FileTransfer{
					Path: filePath,
					Size: fileSize,
					State: DoBlock,
					BlockSize: nr,
					BlockNumber: parts,
					Blocks: totalBlocks,
					DoneBlocks: parts-1,
					DoneSize: int64(cc.BlockSize)*(parts-1),
				})
			}

			ticket, err := cc.putDataBlock(putURL, buffer[:nr], uploadJob.Token)
			if err != nil {
				if cc.MaxRetry <= 0 {
					return fmt.Errorf("cannot push block %d: %v", parts, err)
				}

				for i := 0; i < cc.MaxRetry; i++ {
					if cc.transferProgressHook != nil {
						cc.transferProgressHook(&FileTransfer{
							Path: filePath,
							Size: fileSize,
							State: RetryBlock,
							BlockSize: nr,
							BlockNumber: parts,
							Blocks: totalBlocks,
							DoneBlocks: parts-1,
							DoneSize: int64(cc.BlockSize)*(parts-1),
							Retry: i+1,
							RetriesLeft: cc.MaxRetry-i-1,
							Error: err,
						})
					}

					ticket, err = cc.putDataBlock(putURL, buffer[:nr], uploadJob.Token)
					if err == nil {
						break
					}
				}
			}
			if err != nil {
				return fmt.Errorf("cannot push block %d: %v", parts, err)
			}
			if ticket == "" {
				return fmt.Errorf("missing block %d ticket: %s", parts, filePath)
			}

			if cc.transferProgressHook != nil {
				cc.transferProgressHook(&FileTransfer{
					Path: filePath,
					Size: fileSize,
					State: DoneBlock,
					BlockSize: nr,
					BlockNumber: parts,
					Blocks: totalBlocks,
					DoneBlocks: parts,
					DoneSize: int64(cc.BlockSize)*(parts-1)+int64(nr),
				})
			}

			hashmap[parts] = ticket
		}
	}
	_ = uploadFile.Close()

	fileBlocks := []fileBlockSlek{}
	okBlocks := int64(0)
	for i := int64(1); i <= parts; i++ {
		ticket, ok := hashmap[i]
		// this shouldn't happen
		if !ok {
			return fmt.Errorf("missing block %d: %s", i, filePath)
		}

		okBlocks++
		fileBlocks = append(fileBlocks, fileBlockSlek{
			ETag: ticket,
			Part: i,
		})
	}

	if cc.transferProgressHook != nil {
		cc.transferProgressHook(&FileTransfer{
			Path: filePath,
			Size: fileSize,
			State: ConfirmUpload,
			Blocks: okBlocks,
			DoneBlocks: parts,
			DoneSize: fileSize,
		})
	}

	err = cc.finishFileUpload(uploadJob, fi, &fileBlocks)
	if err != nil {
		return fmt.Errorf("cannot finish upload: %v", err)
	}

	if cc.transferProgressHook != nil {
		cc.transferProgressHook(&FileTransfer{
			Path: filePath,
			Size: fileSize,
			State: FinishTransfer,
			Blocks: okBlocks,
			DoneBlocks: parts,
			DoneSize: fileSize,
		})
	}
	return nil
}

// uploadFileBlocksSerial uploads a file many blocks at a time.
func (cc *CowClient) uploadFileBlocksParallel(filePath string, session *uploadSessionResponse) error {
	fi, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot read file %s: %v", filePath, err)
	}
	// estimate the total number of blocks to upload
	fileSize := fi.Size()
	totalBlocks := blocksInFile(fileSize, cc.BlockSize)

	if cc.transferProgressHook != nil {
		cc.transferProgressHook(&FileTransfer{
			Path: filePath,
			State: InitTransfer,
			Size: fileSize,
			Blocks: totalBlocks,
		})
	}

	uploadJob, err := cc.newFileUpload(fi, session)
	if err != nil {
		return err
	}

	uploadFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file %s: %v", filePath, err)
	}

	wg := new(sync.WaitGroup)
	hashmap := int64map{}

	uploadChan := make(chan *fileBlockUpload)
	for i := 0; i < cc.MaxPushBlocks; i++ {
		go cc.uploadFileBlock(&uploadChan, wg, uploadJob, &hashmap)
	}

	parts := int64(0)
	for {
		buffer := make([]byte, cc.BlockSize)
		nr, err := uploadFile.Read(buffer)
		if nr <= 0 || err != nil {
			// #todo handle err
			break
		}
		parts++
		if nr > 0 {
			wg.Add(1)
			uploadChan <- &fileBlockUpload{
				content: buffer[:nr],
				count: parts,
				filePath: filePath,
				fileSize: fileSize,
				totalBlocks: totalBlocks,
			}
		}
	}

	wg.Wait()
	close(uploadChan)
	_ = uploadFile.Close()

	fileBlocks := []fileBlockSlek{}
	okBlocks := int64(0)
	for i := int64(1); i <= parts; i++ {
		ticket, err, ok := hashmap.Load(i)
		if !ok {
			return fmt.Errorf("missing block %d: %s", i, filePath)
		}
		if err != nil {
			return fmt.Errorf("error pushing block %d: %v", i, err)
		}
		if ticket == "" {
			return fmt.Errorf("missing block %d ticket: %s", i, filePath)
		}

		fileBlocks = append(fileBlocks, fileBlockSlek{
			ETag: ticket,
			Part: i,
		})
	}

	if cc.transferProgressHook != nil {
		cc.transferProgressHook(&FileTransfer{
			Path: filePath,
			Size: fileSize,
			State: ConfirmUpload,
			Blocks: okBlocks,
			DoneBlocks: parts,
			DoneSize: fileSize,
		})
	}

	err = cc.finishFileUpload(uploadJob, fi, &fileBlocks)
	if err != nil {
		return fmt.Errorf("cannot finish upload: %v", err)
	}

	if cc.transferProgressHook != nil {
		cc.transferProgressHook(&FileTransfer{
			Path: filePath,
			Size: fileSize,
			State: FinishTransfer,
			Blocks: okBlocks,
			DoneBlocks: parts,
			DoneSize: fileSize,
		})
	}
	return nil
}

// uploadFileBlock should run as a goroutine. It calls putDataBlock to upload 
// file parts (blocks) to the OSS block upload endpoint.
func (cc *CowClient) uploadFileBlock(ch *chan *fileBlockUpload, wg *sync.WaitGroup, job *ossInitUploadResponse, hashmap *int64map) {
	for item := range *ch {
		putURL := fmt.Sprintf(ossPushBlockURL, cc.OSSURL, job.EncodeID, job.ID, item.count)

		doneBlocks := int64(0)
		doneSize := int64(0)

		if cc.transferProgressHook != nil {
			doneBlocks, doneSize = hashmap.Size()

			cc.transferProgressHook(&FileTransfer{
				Path: item.filePath,
				Size: item.fileSize,
				State: DoBlock,
				BlockSize: len(item.content),
				BlockNumber: item.count,
				Blocks: item.totalBlocks,
				DoneBlocks: doneBlocks,
				DoneSize: doneSize,
			})
		}

		ticket, err := cc.putDataBlock(putURL, item.content, job.Token)
		if err != nil && cc.MaxRetry > 0 {
			for i := 0; i < cc.MaxRetry; i++ {
				if cc.transferProgressHook != nil {
					cc.transferProgressHook(&FileTransfer{
						Path: item.filePath,
						Size: item.fileSize,
						State: RetryBlock,
						BlockSize: len(item.content),
						BlockNumber: item.count,
						Blocks: item.totalBlocks,
						DoneBlocks: doneBlocks,
						DoneSize: doneSize,
						Retry: i+1,
						RetriesLeft: cc.MaxRetry-i-1,
						Error: err,
					})
				}

				ticket, err = cc.putDataBlock(putURL, item.content, job.Token)
				if err == nil {
					break
				}
			}
		}
		if err != nil {
			hashmap.StoreError(item.count, err)
		} else {
			if cc.transferProgressHook != nil {
				cc.transferProgressHook(&FileTransfer{
					Path: item.filePath,
					Size: item.fileSize,
					State: DoneBlock,
					BlockSize: len(item.content),
					BlockNumber: item.count,
					Blocks: item.totalBlocks,
					DoneBlocks: doneBlocks+1,
					DoneSize: doneSize+int64(len(item.content)),
				})
			}

			hashmap.Store(item.count, ticket, len(item.content))
		}
		wg.Done()
	}
}

// newFileUpload calls the file management API to create a file upload 
// operation. It then calls the OSS blocks upload init endpoint to create a 
// blocks upload job.
func (cc *CowClient) newFileUpload(fi os.FileInfo, session *uploadSessionResponse) (*ossInitUploadResponse, error) {
	// first signal to uploadFileURL API that we want to upload a file
	data := map[string]string{
		"fileId":        "",
		"type":          "",
		"fileName":      fi.Name(),
		"originalName":  fi.Name(),
		"fileSize":      strconv.FormatInt(fi.Size(), 10),
		"transferGuid":  session.TransferGUID,
		"storagePrefix": session.Prefix,
	}

	responseBytes, err := cc.newMultipartFormRequest(fmt.Sprintf(uploadFileURL, cc.APIURL), data)
	if err != nil {
		return nil, err
	}

	var createFileResponse *uploadFileResponse
	if err = json.Unmarshal(responseBytes, &createFileResponse); err != nil {
		return nil, err
	}
	// #todo
	session.FileGUID = createFileResponse.FileGuid

	// next signal to ossInitPushURL API that we want to push blocks

	data = map[string]string{
		"transferGuid":  session.TransferGUID,
		"storagePrefix": session.Prefix,
	}
	postBody, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	w := urlEncodeBase64(fmt.Sprintf("%s/%s/%s", session.Prefix, session.TransferGUID, fi.Name()))
	initURL := fmt.Sprintf(ossInitPushURL, cc.OSSURL, w)
	responseBytes, err = cc.newFileUploadRequest(initURL, bytes.NewReader(postBody), session.UploadToken, "POST")
	if err != nil {
		return nil, err
	}

	var initResponse *ossInitUploadResponse
	if err = json.Unmarshal(responseBytes, &initResponse); err != nil {
		return nil, err
	}
	initResponse.Token = session.UploadToken
	initResponse.EncodeID = w
	initResponse.TransferGUID = session.TransferGUID
	initResponse.FileGUID = session.FileGUID

	return initResponse, nil
}

// finishFileUpload calls the OSS merge blocks API, followed by the file 
// management API to signal that the file has been uploaded.
func (cc *CowClient) finishFileUpload(job *ossInitUploadResponse, fi os.FileInfo, sleks *[]fileBlockSlek) error {
	mergeBlocksURL := fmt.Sprintf(ossFinishPushURL, cc.OSSURL, job.EncodeID, job.ID)
	postData := ossMergeBlocksRequest{
		Parts: *sleks,
		FName: fi.Name(),
	}
	postBody, err := json.Marshal(postData)
	if err != nil {
		return err
	}

	reader := bytes.NewReader(postBody)
	resp, err := cc.newFileUploadRequest(mergeBlocksURL, reader, job.Token, "POST")
	if err != nil {
		return err
	}

	var mergeResponse *ossMergeBlocksResponse
	if err = json.Unmarshal(resp, &mergeResponse); err != nil {
		return err
	}

	// now signal to finishUploadFileURL that the file's done
	data := map[string]string{
		"transferGuid": job.TransferGUID,
		"fileGuid":     job.FileGUID,
		"hash":         mergeResponse.Hash,
	}
	bodyBytes, err := cc.newMultipartFormRequest(fmt.Sprintf(finishUploadFileURL, cc.APIURL), data)
	if err != nil {
		return err
	}
	if string(bodyBytes) != "true" {
		return ErrInvalidResponse
	}
	return nil
}

// addHeaders is a helper method to add headers to a HTTP request. These 
// headers are required for interop with Cowtransfer endpoints.
func (cc *CowClient) addHeaders(req *http.Request) *http.Request {
	refererURL := cc.APIURL

	req.Header.Set("Referer", refererURL)
	req.Header.Set("User-Agent", cc.UserAgent)
	req.Header.Set("Origin", refererURL)
	req.Header.Set("Cookie", fmt.Sprintf(defaultCookieFormat, req.Header.Get("Cookie"), time.Now().UnixNano()))
	return req
}

// newFileUploadRequest is a general wrapper for upload related API calls.
func (cc *CowClient) newFileUploadRequest(url string, postBody io.Reader, uploadToken string, httpMethod string) ([]byte, error) {
	refererURL := cc.APIURL

	client := http.Client{Timeout: cc.Timeout}
	req, err := http.NewRequest(httpMethod, url, postBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("referer", refererURL)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "UpToken "+uploadToken)

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

// newMultipartFormRequest is a general wrapper for API calls that require 
// the client to send multipart POST requests.
func (cc *CowClient) newMultipartFormRequest(url string, params map[string]string) ([]byte, error) {
	refererURL := cc.APIURL

	client := http.Client{Timeout: cc.Timeout}
	buffer := &bytes.Buffer{}
	writer := multipart.NewWriter(buffer)
	for key, val := range params {
		_ = writer.WriteField(key, val)
	}
	_ = writer.Close()

	req, err := http.NewRequest("POST", url, buffer)
	if err != nil {
		return nil, err
	}

	req.Header.Set("content-type", fmt.Sprintf("multipart/form-data;boundary=%s", writer.Boundary()))
	req.Header.Set("referer", refererURL)
	req.Header.Set("cookie", cc.Token)

	response, err := client.Do(cc.addHeaders(req))
	if err != nil {
		return nil, err
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	_ = response.Body.Close()

	if s := response.Header.Values("Set-Cookie"); len(s) != 0 && cc.Token == "" {
		for _, v := range s {
			ck := strings.Split(v, ";")
			cc.Token += ck[0] + ";"
		}
	}

	return bodyBytes, nil
}

// putDataBlock uploads buffer as a block to url, which is an OSS block put 
// endpoint.
func (cc *CowClient) putDataBlock(url string, buffer []byte, token string) (string, error) {
	data := new(bytes.Buffer)
	data.Write(buffer)
	body, err := cc.newFileUploadRequest(url, data, token, "PUT")
	if err != nil {
		return "", err
	}

	var uploadResponse ossPushBlockResponse
	if err := json.Unmarshal(body, &uploadResponse); err != nil {
		return "", err
	}

	if cc.VerifyHash {
		if fmt.Sprintf("%x", md5.Sum(buffer)) != uploadResponse.MD5 {
			return "", ErrBlockChecksum
		}
	}
	return uploadResponse.Etag, nil
}
