package httpreq

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type ResponseMethodSuite struct{ suite.Suite }

func (s *ResponseMethodSuite) TestJSONAndBytes() {
	srv := newJSONServer(map[string]string{"foo": "bar"})
	defer srv.Close()
	resp, err := Get(context.Background(), srv.URL, nil)
	s.Require().NoError(err)
	s.NotEmpty(resp.String())
	b := resp.Bytes()
	s.NotEmpty(b)
	var data map[string]string
	s.NoError(resp.JSON(&data))
	s.Equal("bar", data["foo"])
	resp.ClearInternalBuffer()
	s.Empty(resp.Bytes())
}

func (s *ResponseMethodSuite) TestXMLAndDownload() {
	xmlData := "<root><foo>bar</foo></root>"
	srv := newXMLServer(xmlData)
	defer srv.Close()
	resp, err := Get(context.Background(), srv.URL, nil)
	s.Require().NoError(err)
	var v struct {
		Foo string `xml:"foo"`
	}
	s.NoError(resp.XML(&v, nil))
	s.Equal("bar", v.Foo)

	resp2, err := Get(context.Background(), srv.URL, nil)
	s.Require().NoError(err)
	file := "test_download.tmp"
	s.NoError(resp2.DownloadToFile(file))
	defer func() { _ = os.Remove(file) }()
	contents, err := os.ReadFile(file)
	s.NoError(err)
	s.Equal(xmlData, string(contents))
}

type RedirectSuite struct{ suite.Suite }

func (s *RedirectSuite) TestAddRedirectFunctionality() {
	client := &http.Client{}
	ro := &RequestOptions{RedirectLimit: 2, SensitiveHTTPHeaders: map[string]struct{}{"Foo": {}}}
	addRedirectFunctionality(client, ro)
	req1 := httptest.NewRequest("GET", "http://x", nil)
	req1.Header.Set("Foo", "bar")
	req2 := httptest.NewRequest("GET", "http://y", nil)
	s.NoError(client.CheckRedirect(req2, []*http.Request{req1}))
	s.Empty(req2.Header.Get("Foo"))
	req3 := httptest.NewRequest("GET", "http://z", nil)
	err := client.CheckRedirect(req3, []*http.Request{req1, req2})
	s.Equal(ErrRedirectLimitExceeded, err)
}

type SessionSuite struct{ suite.Suite }

func (s *SessionSuite) TestCombineRequestOptions() {
	session, err := NewSession(&RequestOptions{UserAgent: "ua", Headers: map[string]string{"A": "1"}})
	s.Require().NoError(err)
	ro := &RequestOptions{Headers: map[string]string{"B": "2"}}
	out := session.combineRequestOptions(ro)
	s.Equal("ua", out.UserAgent)
	s.Equal("2", out.Headers["B"])
	s.Equal("1", out.Headers["A"])
}

func TestExtraSuites(t *testing.T) {
	suite.Run(t, new(ResponseMethodSuite))
	suite.Run(t, new(RedirectSuite))
	suite.Run(t, new(SessionSuite))
	suite.Run(t, new(InternalFuncsSuite))
}

type InternalFuncsSuite struct{ suite.Suite }

func (s *InternalFuncsSuite) TestEscapeQuotes() {
	in := `"foo\\bar"`
	s.Equal(`\"foo\\\\bar\"`, escapeQuotes(in))
}

func (s *InternalFuncsSuite) TestCreateBasicJSONRequest() {
	ro := &RequestOptions{JSON: map[string]string{"foo": "bar"}}
	req, err := createBasicJSONRequest("POST", "http://x", ro)
	s.NoError(err)
	b, err := io.ReadAll(req.Body)
	s.NoError(err)
	s.JSONEq(`{"foo":"bar"}`, string(b))
	s.Equal("application/json", req.Header.Get("Content-Type"))
}

func (s *InternalFuncsSuite) TestCreateBasicXMLRequest() {
	xmlStr := "<root>ok</root>"
	ro := &RequestOptions{XML: xmlStr}
	req, err := createBasicXMLRequest("POST", "http://x", ro)
	s.NoError(err)
	b, err := io.ReadAll(req.Body)
	s.NoError(err)
	s.Equal(xmlStr, string(b))
	s.Equal("application/xml", req.Header.Get("Content-Type"))
}

func (s *InternalFuncsSuite) TestCreateFileUploadRequest() {
	ro := &RequestOptions{Files: []FileUpload{{FileName: ".txt", FileContents: io.NopCloser(strings.NewReader("data"))}}}
	req, err := createFileUploadRequest("PUT", "http://x", ro)
	s.NoError(err)
	b, err := io.ReadAll(req.Body)
	s.NoError(err)
	s.Equal("data", string(b))
	s.NotEmpty(req.Header.Get("Content-Type"))
}

func (s *InternalFuncsSuite) TestCreateMultiPartPostRequest() {
	ro := &RequestOptions{
		Files: []FileUpload{
			{FileName: "f1.txt", FileContents: io.NopCloser(strings.NewReader("one"))},
			{FileName: "f2.txt", FileContents: io.NopCloser(strings.NewReader("two")), FieldName: "custom"},
		},
		Data: map[string]string{"foo": "bar"},
	}
	req, err := createMultiPartPostRequest("POST", "http://x", ro)
	s.NoError(err)
	mr, err := req.MultipartReader()
	s.NoError(err)
	parts := map[string]string{}
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		s.NoError(err)
		data, err := io.ReadAll(p)
		s.NoError(err)
		parts[p.FormName()] = string(data)
	}
	s.Equal("one", parts["file1"])
	s.Equal("two", parts["custom"])
	s.Equal("bar", parts["foo"])
}

type errReader struct{ err error }

func (e errReader) Read(p []byte) (int, error) { return 0, e.err }
func (e errReader) Close() error               { return nil }

func (s *InternalFuncsSuite) TestCreateMultiPartPostRequestErrors() {
	_, err := createMultiPartPostRequest("POST", "http://x", &RequestOptions{Files: []FileUpload{{FileContents: nil}}})
	s.Error(err)

	_, err = createMultiPartPostRequest("POST", "http://x", &RequestOptions{Files: []FileUpload{{FileName: "f", FileContents: errReader{err: fmt.Errorf("bad")}}}})
	s.Error(err)
}

func (s *InternalFuncsSuite) TestCreateBasicJSONRequestError() {
	_, err := createBasicJSONRequest("POST", "http://x", &RequestOptions{JSON: make(chan int)})
	s.Error(err)
}

func (s *InternalFuncsSuite) TestCreateBasicXMLRequestError() {
	_, err := createBasicXMLRequest("POST", "http://x", &RequestOptions{XML: make(chan int)})
	s.Error(err)
}

func (s *InternalFuncsSuite) TestBuildRequestParamsAndQueryStruct() {
	q := make(chan string, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q <- r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := buildRequest("GET", srv.URL, &RequestOptions{Params: map[string]string{"a": "b"}}, nil)
	s.NoError(err)
	s.Equal("a=b", <-q)

	type qs struct {
		A string `url:"a"`
	}
	_, err = buildRequest("GET", srv.URL, &RequestOptions{QueryStruct: qs{A: "1"}}, nil)
	s.NoError(err)
	s.Equal("a=1", <-q)
}

func (s *InternalFuncsSuite) TestResponseHelpers() {
	resp := &Response{Error: fmt.Errorf("e")}
	n, err := resp.Read(make([]byte, 1))
	s.Equal(-1, n)
	s.Error(err)
	s.EqualError(resp.DownloadToFile("x"), "e")
}

func (s *InternalFuncsSuite) TestProxySettings() {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	ro := RequestOptions{}
	u, err := ro.proxySettings(req)
	s.NoError(err)
	if u != nil {
		s.Contains(u.String(), "proxy")
	}

	proxyURL, _ := url.Parse("http://special")
	ro.Proxies = map[string]*url.URL{"http": proxyURL}
	u, err = ro.proxySettings(req)
	s.NoError(err)
	s.Equal(proxyURL, u)
}

func (s *InternalFuncsSuite) TestAddHTTPHeaders() {
	ro := &RequestOptions{Headers: map[string]string{"X": "Y"}, UserAgent: "ua", Host: "h", Auth: []string{"u", "p"}, IsAjax: true}
	req := httptest.NewRequest("GET", "http://x", nil)
	addHTTPHeaders(ro, req)
	s.Equal("Y", req.Header.Get("X"))
	s.Equal("ua", req.Header.Get("User-Agent"))
	s.Equal("h", req.Host)
	s.Equal("basic", strings.ToLower(req.Header.Get("Authorization")[:5]))
	s.Equal("XMLHttpRequest", req.Header.Get("X-Requested-With"))
}

func (s *InternalFuncsSuite) TestAddCookies() {
	ro := &RequestOptions{Cookies: []*http.Cookie{{Name: "n", Value: "v"}}}
	req := httptest.NewRequest("GET", "http://x", nil)
	addCookies(ro, req)
	s.Equal(1, len(req.Cookies()))
	s.Equal("n", req.Cookies()[0].Name)
}

func (s *InternalFuncsSuite) TestBuildURLStruct() {
	type qs struct {
		A string `url:"a"`
		B []int  `url:"b"`
	}
	out, err := buildURLStruct("http://x", qs{A: "1", B: []int{2, 3}})
	s.NoError(err)
	s.Equal("http://x?a=1&b=2&b=3", out)
}

func (s *InternalFuncsSuite) TestBuildHTTPClient() {
	ro := RequestOptions{}
	hc, err := BuildHTTPClient(ro)
	s.NoError(err)
	s.Equal(http.DefaultClient, hc)

	custom := &http.Client{Timeout: time.Second}
	hc, err = BuildHTTPClient(RequestOptions{HTTPClient: custom})
	s.Equal(custom, hc)
	c, err := BuildHTTPClient(RequestOptions{UseCookieJar: true})
	s.NoError(err)
	s.NotEqual(http.DefaultClient, c)
	s.NotNil(c.Jar)
}

func (s *InternalFuncsSuite) TestCloseIdleConnections() {
	sess, err := NewSession(nil)
	s.Require().NoError(err)
	sess.CloseIdleConnections()
}

func (s *InternalFuncsSuite) TestBuildHTTPClientTracing() {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() { s.NoError(tp.Shutdown(context.Background())) }()

	srv := newGetServer()
	defer srv.Close()

	client, err := BuildHTTPClient(RequestOptions{EnableTracing: true, TracerProvider: tp})
	s.Require().NoError(err)
	s.NotEqual(http.DefaultClient, client)

	req, err := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	s.Require().NoError(err)
	resp, err := client.Do(req)
	s.Require().NoError(err)
	_ = resp.Body.Close()

	spans := exporter.GetSpans()
	s.Require().Len(spans, 1, "otelhttp should have recorded exactly one client span for the round trip")
	s.Equal(oteltrace.SpanKindClient, spans[0].SpanKind)

	// CloseIdleConnections must not panic once Transport is wrapped in
	// *otelhttp.Transport instead of *http.Transport.
	s.NotPanics(func() { client.CloseIdleConnections() })
}

func (s *InternalFuncsSuite) TestBuildHTTPClientTracingGlobalDefault() {
	prev := RequestEnableTracing
	defer func() { RequestEnableTracing = prev }()

	// With the global default off and no per-request opt-in, tracing must
	// not kick in — same behavior as before RequestEnableTracing existed.
	RequestEnableTracing = false
	hc, err := BuildHTTPClient(RequestOptions{})
	s.Require().NoError(err)
	s.Equal(http.DefaultClient, hc)

	// Flipping the global on, with no per-request EnableTracing set, must be
	// enough on its own to enable tracing.
	RequestEnableTracing = true
	hc, err = BuildHTTPClient(RequestOptions{})
	s.Require().NoError(err)
	s.NotEqual(http.DefaultClient, hc)
}
