package worker

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestResolveExecutorHTTPClient_PreservesConfiguredClient(t *testing.T) {
	t.Parallel()

	configured := &http.Client{Timeout: time.Second}

	got := resolveExecutorHTTPClient(ExecutorConfig{HTTPClient: configured})

	if got != configured {
		t.Fatal("resolveExecutorHTTPClient did not preserve configured client")
	}
}

func TestResolveExecutorHTTPClient_Defaults(t *testing.T) {
	t.Parallel()

	client := resolveExecutorHTTPClient(ExecutorConfig{})

	if client.Timeout != defaultExecutorHTTPTimeout {
		t.Fatalf("client timeout = %s, want %s", client.Timeout, defaultExecutorHTTPTimeout)
	}
	if client.CheckRedirect == nil {
		t.Fatal("CheckRedirect is nil")
	}
	if err := client.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect error = %v, want %v", err, http.ErrUseLastResponse)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.MaxIdleConns != 100 {
		t.Fatalf("MaxIdleConns = %d, want 100", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want 10", transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != defaultExecutorIdleConnTimeout {
		t.Fatalf("IdleConnTimeout = %s, want %s", transport.IdleConnTimeout, defaultExecutorIdleConnTimeout)
	}
	if transport.TLSHandshakeTimeout != 10*time.Second {
		t.Fatalf("TLSHandshakeTimeout = %s, want 10s", transport.TLSHandshakeTimeout)
	}
}

func TestResolveExecutorHTTPClient_OverridesTimeouts(t *testing.T) {
	t.Parallel()

	client := resolveExecutorHTTPClient(ExecutorConfig{
		ExecutorHTTPTimeout:     15 * time.Second,
		ExecutorIdleConnTimeout: 20 * time.Second,
	})

	if client.Timeout != 15*time.Second {
		t.Fatalf("client timeout = %s, want 15s", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.IdleConnTimeout != 20*time.Second {
		t.Fatalf("IdleConnTimeout = %s, want 20s", transport.IdleConnTimeout)
	}
}
