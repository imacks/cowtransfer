package cowtransfer_test

import (
	"fmt"
	"github.com/imacks/cowtransfer"
)

func ExampleNewClient() {
	cc := cowtransfer.NewClient()
	
	// default is transfer 1 block at a time.
	// let's do 2 simultaneous block transfers
	cc.MaxPushBlocks = 2

	// you can add hooks to events
	cc.OnStart(func(s *cowtransfer.UploadSession) {
		fmt.Printf("session start\n")
	})
	cc.OnStop(func(s *cowtransfer.UploadSession) {
		fmt.Printf("session stop\n")
	})
	cc.OnFileTransfer(func(fi *cowtransfer.FileTransfer) {
		fmt.Printf("progressing\n")
	})

	// Upload method takes a slice of files. Here we will upload 1 file only.
	dlURL, err := cc.Upload("./testdata/dummy.txt")
	if err != nil {
		panic(err)
	}
	fmt.Println(dlURL)

	// to get the download URL for every file:
	files, err := cc.Files(dlURL)
	if err != nil {
		panic(err)
	}

	for _, v := range files {
		fmt.Printf("%v\n", v)
	}
}
