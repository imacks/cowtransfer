package main

import (
	"flag"
	"fmt"
	"os"
	"time"
	"strings"

	"github.com/imacks/cowtransfer"
)

var (
	token string
	password string
	parallel int
	blksize int
	timeout int
	verbose bool
	verify bool
	maxRetry int
)

func init() {
	flag.StringVar(&token, "c", "", "Cookie token (optional)")
	flag.StringVar(&password, "p", "", "Password (optional)")
	flag.BoolVar(&verbose, "v", false, "Verbose mode")
	flag.BoolVar(&verify, "V", false, "Verify by hash")
	flag.IntVar(&timeout, "T", 10, "Timeout in seconds")
	flag.IntVar(&blksize, "b", 262144, "Block size for uploading")
	flag.IntVar(&parallel, "l", 4, "Parallel threads")
	flag.IntVar(&maxRetry, "r", 3, "Maximum retries")

	flag.Usage = func() {
		fmt.Fprintf(os.Stdout, "%s %s () %s\n", "cowtransfer", "1.0.0", "cowtransfer.cn client")
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintf(os.Stdout, "Usage: %s [<optional-params>] file1 file2... \n", os.Args[0])
		fmt.Fprintf(os.Stdout, "       %s [<optional-params>] url\n", os.Args[0])
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintln(os.Stdout, "Parameters:")
		flag.PrintDefaults()
	}
	flag.Parse()
}

func main() {
	if blksize > 4194304 {
		fmt.Printf("WARNING! Block size too large\n")
		blksize = 524288
	}

	files := flag.Args()
	if len(files) == 0 {
		fmt.Printf("FATAL! files not specified\n")
		os.Exit(1)
	}

	cow := cowtransfer.NewCowClient()
	cow.BlockSize = blksize
	cow.DownloadParallel = parallel
	cow.UploadParallel = parallel
	cow.MaxRetry = maxRetry
	cow.Password = password
	cow.Timeout = time.Duration(timeout) * time.Second
	cow.Token = token
	cow.VerifyHash = verify

	if len(files) == 1 && strings.HasPrefix(files[0], "https://") {
		fi, err := cow.Files(files[0])
		if err != nil {
			panic(err)
		}

		for i, v := range fi {
			fmt.Fprintf(os.Stdout, "idx:  %d\n", i)
			fmt.Fprintf(os.Stdout, "file: %s\n", v.FileName)
			fmt.Fprintf(os.Stdout, "url:  %s\n", v.URL)
			fmt.Fprintf(os.Stdout, "size: %d\n", v.Size)
			if v.Error != nil {
				fmt.Fprintf(os.Stdout, "err:  %s\n", v.Error.Error())
			}

			fmt.Fprintf(os.Stdout, "\n")
		}

		return
	}

	var f []string
	for _, v := range files {
		if strings.HasPrefix(v, "https://") {
			fmt.Printf("FATAL! upload supports local files only\n")
			os.Exit(1)
		}
		f = append(f, v)
	}

	dl, err := cow.Upload(f)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stdout, "link: %s\n", dl)
}