package httpreq

import (
	"context"
	"maps"
	"net/http"
)

// Session makes use of persistent cookies across HTTP requests.
type Session struct {
	// RequestOptions is global options for this session.
	RequestOptions *RequestOptions

	// HTTPClient is the client that will be used to perform this session's
	// requests.
	HTTPClient *http.Client
}

// NewSession creates a new Session which can be used to maintain state across
// requests.
// This function unconditionally sets UseCookieJar as that is the purpose of
// leveraging this facility.
func NewSession(ro *RequestOptions) (*Session, error) {
	if ro == nil {
		ro = &RequestOptions{}
	} else {
		ro = ro.clone()
	}

	ro.UseCookieJar = true

	hc, err := BuildHTTPClient(*ro)
	if err != nil {
		return nil, err
	}

	return &Session{RequestOptions: ro, HTTPClient: hc}, nil
}

func (s *Session) combineRequestOptions(ro *RequestOptions) *RequestOptions {
	if ro == nil {
		ro = &RequestOptions{}
	} else {
		ro = ro.clone()
	}

	if ro.UserAgent == "" && s.RequestOptions.UserAgent != "" {
		ro.UserAgent = s.RequestOptions.UserAgent
	}

	if ro.Host == "" && s.RequestOptions.Host != "" {
		ro.Host = s.RequestOptions.Host
	}

	if ro.Auth == nil && s.RequestOptions.Auth != nil {
		ro.Auth = s.RequestOptions.Auth
	}

	if len(s.RequestOptions.Headers) > 0 || len(ro.Headers) > 0 {
		headers := make(map[string]string)
		maps.Copy(headers, s.RequestOptions.Headers)
		maps.Copy(headers, ro.Headers)
		ro.Headers = headers
	}

	return ro
}

// Get takes a context, url, and optional options, executing the request through
// the current Session.
func (s *Session) Get(ctx context.Context, url string, ro *RequestOptions) (*Response, error) {
	ro = s.combineRequestOptions(ro)
	if ctx != nil {
		ro.Context = ctx
	}
	return doSessionRequest("GET", url, ro, s.HTTPClient)
}

// Put takes a context, url, and optional options, executing the request through
// the current Session.
func (s *Session) Put(ctx context.Context, url string, ro *RequestOptions) (*Response, error) {
	ro = s.combineRequestOptions(ro)
	if ctx != nil {
		ro.Context = ctx
	}
	return doSessionRequest("PUT", url, ro, s.HTTPClient)
}

// Patch takes a context, url, and optional options, executing the request through
// the current Session.
func (s *Session) Patch(ctx context.Context, url string, ro *RequestOptions) (*Response, error) {
	ro = s.combineRequestOptions(ro)
	if ctx != nil {
		ro.Context = ctx
	}
	return doSessionRequest("PATCH", url, ro, s.HTTPClient)
}

// Delete takes a context, url, and optional options, executing the request through
// the current Session.
func (s *Session) Delete(ctx context.Context, url string, ro *RequestOptions) (*Response, error) {
	ro = s.combineRequestOptions(ro)
	if ctx != nil {
		ro.Context = ctx
	}
	return doSessionRequest("DELETE", url, ro, s.HTTPClient)
}

// Post takes a context, url, and optional options, executing the request through
// the current Session.
func (s *Session) Post(ctx context.Context, url string, ro *RequestOptions) (*Response, error) {
	ro = s.combineRequestOptions(ro)
	if ctx != nil {
		ro.Context = ctx
	}
	return doSessionRequest("POST", url, ro, s.HTTPClient)
}

// Head takes a context, url, and optional options, executing the request through
// the current Session.
func (s *Session) Head(ctx context.Context, url string, ro *RequestOptions) (*Response, error) {
	ro = s.combineRequestOptions(ro)
	if ctx != nil {
		ro.Context = ctx
	}
	return doSessionRequest("HEAD", url, ro, s.HTTPClient)
}

// Options takes a context, url, and optional options, executing the request through
// the current Session.
func (s *Session) Options(ctx context.Context, url string, ro *RequestOptions) (*Response, error) {
	ro = s.combineRequestOptions(ro)
	if ctx != nil {
		ro.Context = ctx
	}
	return doSessionRequest("OPTIONS", url, ro, s.HTTPClient)
}

// CloseIdleConnections closes the sessions' idle connections, if any.
//
// This goes through http.Client.CloseIdleConnections rather than asserting
// the Transport is a *http.Transport directly, since RequestOptions.EnableTracing
// (and any custom RequestOptions.HTTPClient) may wrap it in another
// http.RoundTripper.
func (s *Session) CloseIdleConnections() {
	s.HTTPClient.CloseIdleConnections()
}
