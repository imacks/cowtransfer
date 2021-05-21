package cowtransfer

import (
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// listFilesInPath returns a list of file paths. All paths returned are 
// guaranteed to be regular file paths that exists. If fspath contain 
// directories, these directories are walked recursively for regular files. 
// This method resolves symlinks.
func listFilesInPath(fspath ...string) ([]string, int64, error) {
	totalSize := int64(0)

	allFilePaths := []string{}
	for _, v := range fspath {
		fi, err := os.Stat(v)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, -1, fmt.Errorf("path not found: %s", v)
			} else {
				return nil, -1, fmt.Errorf("cannot stat %s: %v", v, err)
			}
		}

		// append if it's just a regular file
		if !fi.IsDir() {
			if !fi.Mode().IsRegular() {
				return nil, -1, fmt.Errorf("only directory or regular file is allowed: %s", v)
			}
			totalSize += fi.Size()
			allFilePaths = append(allFilePaths, v)
			continue
		}

		// v is a dir, so walk recursively to get all files
		err = filepath.Walk(v, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if fi.IsDir() {
				return nil
			}

			if !fi.Mode().IsRegular() {
				return fmt.Errorf("only directory or regular file is allowed: %s", v)
			}

			totalSize += fi.Size()
			allFilePaths = append(allFilePaths, path)
			return nil
		})

		if err != nil {
			return nil, -1, fmt.Errorf("cannot recursively stat %s: %v", v, err)
		}
	}

	return allFilePaths, totalSize, nil
}

func urlEncodeBase64(data string) string {
	r := base64.StdEncoding.EncodeToString([]byte(data))
	r = strings.Replace(r, "+", "-", -1)
	r = strings.Replace(r, "/", "_", -1)

	return r
}

func blocksInFile(filesize int64, blocksize int) int64 {
	blocks, remainder := math.Modf(float64(filesize)/float64(blocksize))
	if remainder == 0 {
		return int64(blocks)
	}
	return int64(blocks)+1
}
