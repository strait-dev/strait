package strait

import (
	"net/http"
	"time"
)

// MiddlewareRequestContext contains information available to the OnRequest hook.
type MiddlewareRequestContext struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

// MiddlewareResponseContext contains information available to the OnResponse hook.
type MiddlewareResponseContext struct {
	Method     string
	URL        string
	Status     int
	DurationMs int64
}

// MiddlewareErrorContext contains information available to the OnError hook.
type MiddlewareErrorContext struct {
	Method string
	URL    string
	Err    error
}

// Middleware defines hooks for intercepting HTTP requests, responses, and errors.
type Middleware struct {
	OnRequest  func(ctx MiddlewareRequestContext)
	OnResponse func(ctx MiddlewareResponseContext)
	OnError    func(ctx MiddlewareErrorContext)
}

type middlewareTransport struct {
	base       http.RoundTripper
	middleware []Middleware
}

func (t *middlewareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for _, mw := range t.middleware {
		if mw.OnRequest != nil {
			mw.OnRequest(MiddlewareRequestContext{
				Method:  req.Method,
				URL:     req.URL.String(),
				Headers: req.Header.Clone(),
			})
		}
	}

	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		for _, mw := range t.middleware {
			if mw.OnError != nil {
				mw.OnError(MiddlewareErrorContext{
					Method: req.Method,
					URL:    req.URL.String(),
					Err:    err,
				})
			}
		}
		return nil, err
	}

	for _, mw := range t.middleware {
		if mw.OnResponse != nil {
			mw.OnResponse(MiddlewareResponseContext{
				Method:     req.Method,
				URL:        req.URL.String(),
				Status:     resp.StatusCode,
				DurationMs: durationMs,
			})
		}
	}

	return resp, nil
}

func wrapTransportWithMiddleware(base http.RoundTripper, middleware []Middleware) http.RoundTripper {
	if len(middleware) == 0 {
		return base
	}
	return &middlewareTransport{base: base, middleware: middleware}
}
