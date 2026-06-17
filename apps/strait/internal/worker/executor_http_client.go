package worker

import (
	"net/http"
	"time"

	"strait/internal/httputil"
)

const (
	defaultExecutorHTTPTimeout     time.Duration = 300_000_000_000
	defaultExecutorIdleConnTimeout time.Duration = 90_000_000_000
)

func resolveExecutorHTTPClient(cfg ExecutorConfig) *http.Client {
	if cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}

	execTimeout := cfg.ExecutorHTTPTimeout
	if execTimeout <= 0 {
		execTimeout = defaultExecutorHTTPTimeout
	}
	execIdleTimeout := cfg.ExecutorIdleConnTimeout
	if execIdleTimeout <= 0 {
		execIdleTimeout = defaultExecutorIdleConnTimeout
	}

	transport := httputil.NewExternalTransport(cfg.AllowPrivateEndpoints)
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = execIdleTimeout
	transport.TLSHandshakeTimeout = 10 * time.Second

	return &http.Client{
		Timeout:   execTimeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
