package httpreq

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
)

type PatchSuite struct {
	suite.Suite
}

func (s *PatchSuite) TestPatchRequest() {
	srv := newPatchServer()
	defer srv.Close()

	resp, err := Patch(context.Background(), srv.URL, &RequestOptions{Data: map[string]string{"one": "two"}})
	s.Require().NoError(err)
	s.True(resp.OK)
}

func (s *PatchSuite) TestPatchInvalidURLSession() {
	session, err := NewSession(nil)
	s.Require().NoError(err)
	_, err = session.Patch(context.Background(), "%../dir/", nil)
	s.Error(err)
}

func TestPatchSuite(t *testing.T) {
	suite.Run(t, new(PatchSuite))
}
