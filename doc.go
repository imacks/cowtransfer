/*
Package cowtransfer is a client API for cowtransfer.cn.

The file upload workflow involves opening up a session, upload one or more 
files, and finally closing the session. Files are not uploaded to Cowtransfer 
site directly, but to Qiniu object storage webservice.

Qiniu object storage webservice expects a file to be uploaded in multiple 
blocks. It offers APIs to signal the beginning of a file upload, upload of a 
block that will be associated with the file, and merging of blocks.

This package offers the ability to upload blocks with multi-threading by 
setting CowClient.MaxPushBlocks. This may not be faster than single threaded 
upload due to timeouts and retries.
*/
package cowtransfer