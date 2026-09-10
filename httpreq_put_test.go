package httpreq

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
)

type PutSuite struct {
	suite.Suite
}

func (s *PutSuite) TestPutRequest() {
	srv := newPutServer()
	defer srv.Close()

	resp, err := Put(context.Background(), srv.URL, &RequestOptions{Data: map[string]string{"one": "two"}})
	s.Require().NoError(err)
	s.True(resp.OK)
}

func (s *PutSuite) TestPutInvalidURLSession() {
	session, err := NewSession(nil)
	s.Require().NoError(err)
	_, err = session.Put(context.Background(), "%../dir/", nil)
	s.Error(err)
}

func TestPutSuite(t *testing.T) {
	suite.Run(t, new(PutSuite))
}
