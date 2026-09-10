// Package httpreq implements a friendly API over Go's existing net/http
// library. It is heavily based on the archived github.com/levigross/grequests;
// see the NOTICE file at the root of this module for attribution.
package httpreq

import "context"

// Get takes a context, an url, and optional options, performs the request and
// returns its Response.
func Get(ctx context.Context, url string, options *RequestOptions) (*Response, error) {
	return Request(ctx, "GET", url, options)
}

// Put takes a context, an url, and optional options, performs the request and
// returns its Response.
func Put(ctx context.Context, url string, options *RequestOptions) (*Response, error) {
	return Request(ctx, "PUT", url, options)
}

// Patch takes a context, an url, and optional options, performs the request and
// returns its Response.
func Patch(ctx context.Context, url string, options *RequestOptions) (*Response, error) {
	return Request(ctx, "PATCH", url, options)
}

// Delete takes a context, an url, and optional options, performs the request
// and returns its Response.
func Delete(ctx context.Context, url string, options *RequestOptions) (*Response, error) {
	return Request(ctx, "DELETE", url, options)
}

// Post takes a context, an url, and optional options, performs the request and
// returns its Response.
func Post(ctx context.Context, url string, options *RequestOptions) (*Response, error) {
	return Request(ctx, "POST", url, options)
}

// Head takes a context, an url, and optional options, performs the request and
// returns its Response.
func Head(ctx context.Context, url string, options *RequestOptions) (*Response, error) {
	return Request(ctx, "HEAD", url, options)
}

// Options takes a context, an url, and optional options, performs the request
// and returns its Response.
func Options(ctx context.Context, url string, options *RequestOptions) (*Response, error) {
	return Request(ctx, "OPTIONS", url, options)
}

// Request takes a context, an HTTP verb, an url, and optional options, performs
// the request and returns its Response.
func Request(ctx context.Context, verb, url string, options *RequestOptions) (*Response, error) {
	ro := options
	if ro == nil {
		ro = &RequestOptions{}
	}
	if ctx != nil {
		ro = ro.clone()
		ro.Context = ctx
	}
	return doRegularRequest(verb, url, ro)
}
