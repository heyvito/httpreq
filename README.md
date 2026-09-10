# httpreq

httpreq provides an abstraction API over Go's `net/http`. One-shot verb funcs 
(`Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, `Options`), a 
`RequestOptions` struct covering the usual pain points (JSON/XML/form bodies, 
multipart uploads, query structs, proxies, cookie jars, TLS/dial/request 
timeouts, redirect handling, OTel tracing), and `Session` for cookie-jar-backed 
request reuse.

Heavily based on [levigross/grequests](https://github.com/levigross/grequests)
(archived); see [NOTICE](NOTICE).

```go
go get github.com/heyvito/httpreq
```

## Usage

```go
resp, err := httpreq.Get(ctx, "https://api.example.com/things", nil)
if err != nil {
    // network/transport error; resp.Error is also set
}
defer resp.Close()

if resp.OK { // 2xx
    var things []Thing
    if err := resp.JSON(&things); err != nil {
        // ...
    }
}
```

`Response` also exposes `String()`, `Bytes()`, `XML(&v, charsetReader)`,
`DownloadToFile(path)`, and satisfies `io.Reader`. `String`/`Bytes` buffer the
body internally so they can be called more than once; `JSON`/`XML`/
`DownloadToFile` close the response when done.

### Request bodies

```go
httpreq.Post(ctx, url, &httpreq.RequestOptions{JSON: payload})
httpreq.Post(ctx, url, &httpreq.RequestOptions{XML: payload})
httpreq.Post(ctx, url, &httpreq.RequestOptions{Data: map[string]string{"a": "b"}}) // form-urlencoded
httpreq.Post(ctx, url, &httpreq.RequestOptions{RequestBody: someReader})           // takes precedence over the above
```

`JSON`/`XML` accept `string`, `[]byte`, or any marshalable value.

### Query params

```go
&httpreq.RequestOptions{Params: map[string]string{"q": "1"}}
// or, mutually exclusive with Params:
&httpreq.RequestOptions{QueryStruct: myStruct} // github.com/google/go-querystring tags
```

### File uploads

```go
files, _ := httpreq.FileUploadFromDisk("report.pdf")
httpreq.Post(ctx, url, &httpreq.RequestOptions{Files: files, Data: map[string]string{"note": "hi"}})
```

`FileUploadFromGlob` builds `[]FileUpload` from a glob pattern, skipping
files that fail to stat. Multipart (`POST`) accepts multiple files; `PUT`/
`PATCH` send the raw body and accept exactly one (`ErrInvalidFileAmount`
otherwise).

### Sessions

```go
sess, _ := httpreq.NewSession(&httpreq.RequestOptions{UserAgent: "my-client/1.0"})
resp, err := sess.Get(ctx, url, nil) // shares cookie jar + client across calls
sess.CloseIdleConnections()
```

Per-call `RequestOptions` passed to a session method are merged on top of the
session's defaults (`UserAgent`, `Host`, `Auth`, `Headers`).

### Other options worth knowing about

- `InsecureSkipVerify`, `Proxies map[scheme]*url.URL`, `LocalAddr`,
  `DialTimeout`/`DialKeepAlive`/`TLSHandshakeTimeout`/`RequestTimeout`,
  `HTTPClient` (bypasses all of the above), `BeforeRequest func(*http.Request) error`.
- `RedirectLimit` (default 30, global `RequestRedirectLimit`) and
  `SensitiveHTTPHeaders` (default `RequestSensitiveHTTPHeaders`: Authorization,
  Proxy-Authorization, Www-Authenticate) control redirect behaviour. Sensitive
  headers are stripped on cross-request redirects, everything else is
  forwarded.
- `EnableTracing` wraps the transport with `otelhttp`, producing a client span
  per round trip (incl. redirects) and injecting trace context from `ctx`.
  `TracerProvider` overrides the otelhttp default. Set the package var
  `RequestEnableTracing = true` to enable it process-wide.

## License

Apache License 2.0 — see [LICENSE](LICENSE). Portions derived from
[grequests](https://github.com/levigross/grequests), Copyright 2015 Levi
Gross; see [NOTICE](NOTICE) for attribution.
