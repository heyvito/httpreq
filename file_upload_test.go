package httpreq

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type FileUploadSuite struct {
	suite.Suite
}

func (s *FileUploadSuite) TestErrorOpenFile() {
	fd, err := FileUploadFromDisk("I am Not A File")
	s.Error(err)
	s.Nil(fd)
}

func (s *FileUploadSuite) TestGLOBFiles() {
	fd, err := FileUploadFromGlob("fixtures/*")
	s.NoError(err)
	s.Len(fd, 2)
}

func (s *FileUploadSuite) TestInvalidGlob() {
	_, err := FileUploadFromGlob("[-]")
	s.Error(err)
}

func (s *FileUploadSuite) TestNoGlobFiles() {
	_, err := FileUploadFromGlob("notapath")
	s.Error(err)
}

func (s *FileUploadSuite) TestGlobWithDir() {
	fd, err := FileUploadFromGlob("*test*")
	s.NoError(err)

	for _, f := range fd {
		s.NotEqual("test_files", f.FileName, "directories cannot be uploaded")
	}
}

func TestFileUploadSuite(t *testing.T) {
	suite.Run(t, new(FileUploadSuite))
}
