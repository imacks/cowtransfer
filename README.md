cowtransfer
===========
This is a client for https://cowtransfer.cn

Most code is from https://github.com/Mikubill/cowtransfer-uploader. I have removed dependency on 
`github.com/cheggaaa/pb` in favor of various progress event hooks. The intention is to decouple 
the CLI from the library API. 
