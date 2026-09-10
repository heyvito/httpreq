package httpreq

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// FileUpload is a struct that is used to specify the file that a User
// wishes to upload.
type FileUpload struct {
	// Filename is the name of the file to upload. This is used to guess the mimetype as well as pass it onto the server
	FileName string

	// FileContents just needs to be an io.ReadCloser (which most files satisfy anyway)
	FileContents io.ReadCloser

	// FieldName is form field name
	FieldName string

	// FileMime represents which mimetime should be sent along with the file.
	// When empty, defaults to application/octet-stream
	FileMime string
}

// FileUploadFromDisk creates a FileUpload struct slice from a location on disk
func FileUploadFromDisk(fileName string) ([]FileUpload, error) {
	fd, err := os.Open(fileName)

	if err != nil {
		return nil, err
	}

	return []FileUpload{{FileContents: fd, FileName: fileName}}, nil

}

// FileUploadFromGlob creates a FileUpload struct slice from a glob location on disk
// this function will gloss over all errors in the files and only upload the files that don't return errors from the glob
func FileUploadFromGlob(fileSystemGlob string) ([]FileUpload, error) {
	files, err := filepath.Glob(fileSystemGlob)

	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, errors.New("httpreq: no files have been returned in the glob")
	}

	filesToUpload := make([]FileUpload, 0, len(files))

	for _, f := range files {
		if s, err := os.Stat(f); err != nil || s.IsDir() {
			continue
		}

		// ignoring error because I can stat the file
		fd, _ := os.Open(f)

		filesToUpload = append(filesToUpload, FileUpload{FileContents: fd, FileName: filepath.Base(fd.Name())})

	}

	return filesToUpload, nil

}
