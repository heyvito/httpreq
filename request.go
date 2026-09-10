package httpreq

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json/v2"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	stdurl "net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/google/go-querystring/query"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	// Default value for net.Dialer Timeout
	dialTimeout = 30 * time.Second

	// Default value for net.Dialer KeepAlive
	dialKeepAlive = 30 * time.Second

	// Default value for http.Transport TLSHandshakeTimeout
	tlsHandshakeTimeout = 10 * time.Second

	// Default value for Request Timeout
	requestTimeout = 90 * time.Second
)

var (
	localUserAgent = "httpreq/0.1.0-" + runtime.Version()

	// ErrRedirectLimitExceeded indicates the response yielded too many
	// redirects.
	ErrRedirectLimitExceeded = errors.New("httpreq: request exceeded redirect count")

	// ErrInvalidFileAmount indicates that more than one file was provided to a
	// PATCH or PUT request, which is not supported.
	ErrInvalidFileAmount = errors.New("httpreq: PUT and PATCH requests supports at most one file")

	// RequestRedirectLimit is a tunable variable that specifies how many times
	// redirects can be made in response to a redirect. This is the global
	// variable; to set this on a request by request basis, set it within the
	// `RequestOptions` structure.
	RequestRedirectLimit = 30

	// RequestEnableTracing is a tunable variable that, when true, enables
	// OpenTelemetry tracing (see RequestOptions.EnableTracing) on every
	// request regardless of what that particular request's RequestOptions
	// set. This is the global variable, meant to be set once at startup so
	// individual call sites don't each need `EnableTracing: true`; a request
	// can still set `RequestOptions.EnableTracing = true` on its own even
	// when this is false, but — since a bare bool can't tell "unset" apart
	// from "false" — there is no per-request way to opt out while this is
	// true.
	RequestEnableTracing bool

	// RequestSensitiveHTTPHeaders is a map of sensitive HTTP headers that must
	// not be passed on a redirect. This is the global variable; to set this on
	// a request by request basis, set it within the `RequestOptions` structure.
	RequestSensitiveHTTPHeaders = map[string]struct{}{
		"Www-Authenticate":    {},
		"Authorization":       {},
		"Proxy-Authorization": {},
	}
)

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string { return quoteEscaper.Replace(s) }

// RequestOptions holds the set of options that configure a single request
type RequestOptions struct {
	// Data is a map of key values that will eventually convert into the
	// body of a POST request.
	Data map[string]string

	// Params is a map of query strings to be attached to the request.
	Params map[string]string

	// QueryStruct is a struct that encapsulates a set of URL query params
	// this parameter is mutually exclusive with `Params map[string]string`
	// (they cannot be combined). For more information, see
	// https://godoc.org/github.com/google/go-querystring/query
	QueryStruct any

	// Files accepts files to be uploaded. The use of this field is limited to
	// POST requests.
	Files []FileUpload

	// JSON, when set, marshalls its contents as JSON on request body.
	JSON any

	// XML, when set, marshalls its contents as XML on the request body.
	XML any

	// Headers defines custom HTTP headers to the request.
	Headers map[string]string

	// InsecureSkipVerify indicates if the server's TLS certificate must be
	// validated. Note that that Go's TLS verify mechanism doesn't validate if a
	// certificate has been revoked
	InsecureSkipVerify bool

	// DisableCompression, when set, disables gzip compression on this request.
	DisableCompression bool

	// UserAgent sets an arbitrary custom user agent for the request.
	UserAgent string

	// Host sets an arbitrary host for the request.
	Host string

	// Auth specifies a username and password to use with the request. It
	// will use basic HTTP authentication formatting the username and password
	// as specified per the HTTP Protocol Specification, being
	// base64(username ":" password)
	Auth []string

	// IsAjax sets whether the request should carry headers as if it was
	// sent by a browser's JavaScript engine.
	IsAjax bool

	// Cookies attaches the provided array of `http.Cookie` to the request.
	Cookies []*http.Cookie

	// UseCookieJar creates a custom HTTP client that processes and stores HTTP
	// cookies when they are sent down.
	UseCookieJar bool

	// Proxies is a map in the following format:
	// 	*protocol* => proxy address
	// Example:
	//	http => http://127.0.0.1:8080
	Proxies map[string]*stdurl.URL

	// TLSHandshakeTimeout specifies the maximum amount of time to
	// wait for a TLS handshake. Zero means no timeout.
	TLSHandshakeTimeout time.Duration

	// DialTimeout specifies the maximum amount of time a dial will wait for
	// connect to complete.
	DialTimeout time.Duration

	// DialKeepAlive specifies the keep-alive period for an active
	// network connection. If zero, keep-alive are not enabled.
	DialKeepAlive time.Duration

	// RequestTimeout specifies the maximum amount of time a whole request
	// (dial / request / redirects) may take.
	RequestTimeout time.Duration

	// HTTPClient can be provided to supply a custom HTTP client;
	// this is useful for using an OAuth client with the request.
	HTTPClient *http.Client

	// SensitiveHTTPHeaders specifies a map of sensitive HTTP headers that
	// should not be passed on a redirect.
	SensitiveHTTPHeaders map[string]struct{}

	// RedirectLimit specifies the maximum acceptable amount of redirects that
	// we should expect before returning an error; by default this is set to 30.
	// This can be changed globally by modifying the `httpreq.RedirectLimit`
	// variable.
	RedirectLimit int

	// RequestBody accepts anything matching an `io.Reader` for the request
	// body. This option takes precedence over any other request option
	// specified.
	RequestBody io.Reader

	// CookieJar specifies an http.CookieJar to be used with the request.
	// This option takes precedence over `UseCookieJar`.
	CookieJar http.CookieJar

	// Context can be used to maintain state and cancellation between requests.
	Context context.Context

	// BeforeRequest is a hook that allows the request object to be modified
	// before it has been fired. This is useful for adding authentication
	// and other functionality not provided in this library.
	BeforeRequest func(req *http.Request) error

	// LocalAddr specifies which local interface must be used for the request.
	LocalAddr *net.TCPAddr

	// EnableTracing wraps the request's transport with OpenTelemetry
	// instrumentation (go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp),
	// producing a client span per HTTP round trip — including one per redirect
	// hop — and injecting the trace context into outgoing request headers via
	// the configured propagator. The span's parent is read from the context
	// passed to the request (Get, Session.Get, etc.), so it composes with
	// whatever tracing setup already produced that context.
	//
	// Tracing is also enabled when the `RequestEnableTracing` package variable
	// is true, regardless of this field; set that once instead of this field
	// on every request to enable tracing process-wide.
	EnableTracing bool

	// TracerProvider overrides the TracerProvider used when tracing is
	// enabled (see EnableTracing). When nil, otelhttp falls back to the
	// global TracerProvider (otel.GetTracerProvider()). Has no effect when
	// tracing is disabled.
	TracerProvider oteltrace.TracerProvider
}

// tracingEnabled reports whether this request should be traced, honouring
// both the per-request EnableTracing field and the process-wide
// RequestEnableTracing default.
func (ro RequestOptions) tracingEnabled() bool {
	return ro.EnableTracing || RequestEnableTracing
}

// clone returns a shallow copy of ro. Building a request mutates a handful of
// fields (UseCookieJar, Context, RedirectLimit, SensitiveHTTPHeaders) to fill
// in defaults, so cloning first ensures writes never leak back into the
// RequestOptions a caller passed in, which may be reused across requests
// (e.g. a Session) or shared across goroutines.
func (ro RequestOptions) clone() *RequestOptions {
	cp := ro
	return &cp
}

func (ro RequestOptions) proxySettings(req *http.Request) (*stdurl.URL, error) {
	if len(ro.Proxies) == 0 {
		return http.ProxyFromEnvironment(req)
	}

	if _, ok := ro.Proxies[req.URL.Scheme]; ok {
		return ro.Proxies[req.URL.Scheme], nil
	}

	return http.ProxyFromEnvironment(req)
}

// needsCustomHTTPClient indicates whether a custom client is required.
func (ro RequestOptions) needsCustomHTTPClient() bool {
	return ro.InsecureSkipVerify ||
		ro.DisableCompression ||
		len(ro.Proxies) != 0 ||
		ro.TLSHandshakeTimeout != 0 ||
		ro.DialTimeout != 0 ||
		ro.DialKeepAlive != 0 ||
		len(ro.Cookies) != 0 ||
		ro.UseCookieJar ||
		ro.RequestTimeout != 0 ||
		ro.LocalAddr != nil ||
		ro.tracingEnabled()
}

func doRegularRequest(requestVerb, url string, ro *RequestOptions) (*Response, error) {
	return buildResponse(buildRequest(requestVerb, url, ro, nil))
}

func doSessionRequest(requestVerb, url string, ro *RequestOptions, httpClient *http.Client) (*Response, error) {
	return buildResponse(buildRequest(requestVerb, url, ro, httpClient))
}

func buildRequest(httpMethod, url string, ro *RequestOptions, httpClient *http.Client) (*http.Response, error) {
	if ro == nil {
		ro = &RequestOptions{}
	} else {
		ro = ro.clone()
	}

	if ro.CookieJar != nil {
		ro.UseCookieJar = true
	}
	var err error

	if httpClient == nil {
		httpClient, err = BuildHTTPClient(*ro)
	}
	if err != nil {
		return nil, err
	}

	switch {
	case len(ro.Params) != 0:
		if url, err = buildURLParams(url, ro.Params); err != nil {
			return nil, err
		}
	case ro.QueryStruct != nil:
		if url, err = buildURLStruct(url, ro.QueryStruct); err != nil {
			return nil, err
		}
	}

	req, err := buildHTTPRequest(httpMethod, url, ro)

	if err != nil {
		return nil, err
	}

	addHTTPHeaders(ro, req)
	addCookies(ro, req)
	addRedirectFunctionality(httpClient, ro)

	if ro.Context != nil {
		req = req.WithContext(ro.Context)
	}

	if ro.BeforeRequest != nil {
		if err := ro.BeforeRequest(req); err != nil {
			return nil, err
		}
	}

	return httpClient.Do(req)
}

func buildHTTPRequest(httpMethod, url string, ro *RequestOptions) (*http.Request, error) {
	if ro.RequestBody != nil {
		return http.NewRequest(httpMethod, url, ro.RequestBody)
	}

	if ro.JSON != nil {
		return createBasicJSONRequest(httpMethod, url, ro)
	}

	if ro.XML != nil {
		return createBasicXMLRequest(httpMethod, url, ro)
	}

	if ro.Files != nil {
		return createFileUploadRequest(httpMethod, url, ro)
	}

	if ro.Data != nil {
		return createBasicRequest(httpMethod, url, ro)
	}

	return http.NewRequest(httpMethod, url, nil)
}

func createFileUploadRequest(httpMethod, url string, ro *RequestOptions) (*http.Request, error) {
	if httpMethod == "POST" {
		return createMultiPartPostRequest(httpMethod, url, ro)
	}

	if len(ro.Files) > 1 {
		return nil, ErrInvalidFileAmount
	}

	req, err := http.NewRequest(httpMethod, url, ro.Files[0].FileContents)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", mime.TypeByExtension(ro.Files[0].FileName))
	return req, nil

}

func createBasicXMLRequest(httpMethod, url string, ro *RequestOptions) (*http.Request, error) {
	var reader io.Reader

	switch ro.XML.(type) {
	case string:
		reader = strings.NewReader(ro.XML.(string))
	case []byte:
		reader = bytes.NewReader(ro.XML.([]byte))
	default:
		byteSlice, err := xml.Marshal(ro.XML)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(byteSlice)
	}

	req, err := http.NewRequest(httpMethod, url, reader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/xml")

	return req, nil

}

func createMultiPartPostRequest(httpMethod, url string, ro *RequestOptions) (*http.Request, error) {
	requestBody := &bytes.Buffer{}
	multipartWriter := multipart.NewWriter(requestBody)

	for i, f := range ro.Files {
		if f.FileContents == nil {
			return nil, errors.New("httpreq: pointer FileContents cannot be nil")
		}

		fieldName := f.FieldName

		if fieldName == "" {
			if len(ro.Files) > 1 {
				fieldName = strings.Join([]string{"file", strconv.Itoa(i + 1)}, "")
			} else {
				fieldName = "file"
			}
		}

		var writer io.Writer
		var err error

		if f.FileMime != "" {
			if f.FileName == "" {
				f.FileName = "filename"
			}
			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeQuotes(fieldName), escapeQuotes(f.FileName)))
			h.Set("Content-Type", f.FileMime)
			writer, err = multipartWriter.CreatePart(h)
		} else {
			writer, err = multipartWriter.CreateFormFile(fieldName, f.FileName)
		}
		if err != nil {
			return nil, err
		}

		if _, err = io.Copy(writer, f.FileContents); err != nil && err != io.EOF {
			return nil, err
		}
		if err := f.FileContents.Close(); err != nil {
			return nil, err
		}
	}

	for key, value := range ro.Data {
		if err := multipartWriter.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	if err := multipartWriter.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(httpMethod, url, requestBody)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", multipartWriter.FormDataContentType())

	return req, err
}

func createBasicJSONRequest(httpMethod, url string, ro *RequestOptions) (*http.Request, error) {
	var reader io.Reader
	switch ro.JSON.(type) {
	case string:
		reader = strings.NewReader(ro.JSON.(string))
	case []byte:
		reader = bytes.NewReader(ro.JSON.([]byte))
	default:
		byteSlice, err := json.Marshal(ro.JSON)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(byteSlice)
	}

	req, err := http.NewRequest(httpMethod, url, reader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

func createBasicRequest(httpMethod, url string, ro *RequestOptions) (*http.Request, error) {
	req, err := http.NewRequest(httpMethod, url, strings.NewReader(encodePostValues(ro.Data)))

	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return req, nil
}

func encodePostValues(postValues map[string]string) string {
	urlValues := &stdurl.Values{}

	for key, value := range postValues {
		urlValues.Set(key, value)
	}

	return urlValues.Encode()
}

// BuildHTTPClient returns a custom HTTP client based on values provided to
// RequestOptions.
func BuildHTTPClient(ro RequestOptions) (*http.Client, error) {
	if ro.HTTPClient != nil {
		return ro.HTTPClient, nil
	}

	if !ro.needsCustomHTTPClient() {
		return http.DefaultClient, nil
	}

	if ro.TLSHandshakeTimeout == 0 {
		ro.TLSHandshakeTimeout = tlsHandshakeTimeout
	}

	if ro.DialTimeout == 0 {
		ro.DialTimeout = dialTimeout
	}

	if ro.DialKeepAlive == 0 {
		ro.DialKeepAlive = dialKeepAlive
	}

	if ro.RequestTimeout == 0 {
		ro.RequestTimeout = requestTimeout
	}

	var cookieJar http.CookieJar

	if ro.UseCookieJar {
		if ro.CookieJar != nil {
			cookieJar = ro.CookieJar
		} else {
			var err error
			cookieJar, err = cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
			if err != nil {
				return nil, err
			}
		}
	}

	var transport http.RoundTripper = createHTTPTransport(ro)
	if ro.tracingEnabled() {
		var opts []otelhttp.Option
		if ro.TracerProvider != nil {
			opts = append(opts, otelhttp.WithTracerProvider(ro.TracerProvider))
		}
		transport = otelhttp.NewTransport(transport, opts...)
	}

	return &http.Client{
		Jar:       cookieJar,
		Transport: transport,
		Timeout:   ro.RequestTimeout,
	}, nil
}

func createHTTPTransport(ro RequestOptions) *http.Transport {
	ourHTTPTransport := &http.Transport{
		Proxy: ro.proxySettings,
		DialContext: (&net.Dialer{
			Timeout:   ro.DialTimeout,
			KeepAlive: ro.DialKeepAlive,
			LocalAddr: ro.LocalAddr,
		}).DialContext,
		TLSHandshakeTimeout: ro.TLSHandshakeTimeout,

		TLSClientConfig:    &tls.Config{InsecureSkipVerify: ro.InsecureSkipVerify},
		DisableCompression: ro.DisableCompression,
	}
	ensureTransporterFinalized(ourHTTPTransport)
	return ourHTTPTransport
}

func buildURLParams(url string, params map[string]string) (string, error) {
	parsedURL, err := stdurl.Parse(url)
	if err != nil {
		return "", err
	}

	parsedQuery, err := stdurl.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		return "", err
	}

	for key, value := range params {
		parsedQuery.Set(key, value)
	}

	return addQueryParams(parsedURL, parsedQuery), nil
}

func addHTTPHeaders(ro *RequestOptions, req *http.Request) {
	for key, value := range ro.Headers {
		req.Header.Set(key, value)
	}

	ua := localUserAgent

	if ro.UserAgent != "" {
		ua = ro.UserAgent
	}
	req.Header.Set("User-Agent", ua)

	if ro.Host != "" {
		req.Host = ro.Host
	}

	if ro.Auth != nil {
		req.SetBasicAuth(ro.Auth[0], ro.Auth[1])
	}

	if ro.IsAjax {
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
	}
}

func addCookies(ro *RequestOptions, req *http.Request) {
	for _, c := range ro.Cookies {
		req.AddCookie(c)
	}
}

func addQueryParams(parsedURL *stdurl.URL, parsedQuery stdurl.Values) string {
	return strings.Join([]string{
		strings.ReplaceAll(parsedURL.String(), "?"+parsedURL.RawQuery, ""),
		parsedQuery.Encode(),
	}, "?")
}

func buildURLStruct(url string, URLStruct any) (string, error) {
	parsedURL, err := stdurl.Parse(url)

	if err != nil {
		return "", err
	}

	parsedQuery, err := stdurl.ParseQuery(parsedURL.RawQuery)

	if err != nil {
		return "", err
	}

	queryStruct, err := query.Values(URLStruct)
	if err != nil {
		return "", err
	}

	for key, value := range queryStruct {
		for _, v := range value {
			parsedQuery.Add(key, v)
		}
	}

	return addQueryParams(parsedURL, parsedQuery), nil
}

func addRedirectFunctionality(client *http.Client, ro *RequestOptions) {
	if client.CheckRedirect != nil {
		return
	}

	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if ro.RedirectLimit < 0 {
			return http.ErrUseLastResponse
		}

		if ro.RedirectLimit == 0 {
			ro.RedirectLimit = RequestRedirectLimit
		}

		if len(via) >= ro.RedirectLimit {
			return ErrRedirectLimitExceeded
		}

		if ro.SensitiveHTTPHeaders == nil {
			ro.SensitiveHTTPHeaders = RequestSensitiveHTTPHeaders
		}

		for k, vv := range via[0].Header {
			if _, found := ro.SensitiveHTTPHeaders[k]; found {
				continue
			}

			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}

		return nil
	}
}

func ensureTransporterFinalized(httpTransport *http.Transport) {
	runtime.AddCleanup(httpTransport, func(h **http.Transport) {
		if h == nil || *h == nil {
			return
		}
		(*h).CloseIdleConnections()
	}, &httpTransport)
}
