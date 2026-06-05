package worker

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveExecutorHTTPClient_PreservesConfiguredClient(t *testing.T) {
	t.Parallel()

	configured := &http.Client{Timeout: time.Second}

	got := resolveExecutorHTTPClient(ExecutorConfig{HTTPClient: configured})
	require.Equal(t,
		configured,
		got)

}

func TestResolveExecutorHTTPClient_Defaults(t *testing.T) {
	t.Parallel()

	client := resolveExecutorHTTPClient(ExecutorConfig{})
	require.Equal(t,
		defaultExecutorHTTPTimeout,

		client.
			Timeout)
	require.NotNil(
		t, client.CheckRedirect,
	)

	if err := client.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		require.Failf(t, "test failure",

			"CheckRedirect error = %v, want %v", err, http.ErrUseLastResponse)
	}

	transport, ok := client.Transport.(*http.Transport)
	require.True(t,
		ok)
	require.EqualValues(t, 100, transport.
		MaxIdleConns,
	)
	require.EqualValues(t, 10, transport.
		MaxIdleConnsPerHost,
	)
	require.Equal(t,
		defaultExecutorIdleConnTimeout,

		transport.IdleConnTimeout)
	require.Equal(t,
		10*time.
			Second, transport.
			TLSHandshakeTimeout,
	)

}

func TestResolveExecutorHTTPClient_OverridesTimeouts(t *testing.T) {
	t.Parallel()

	client := resolveExecutorHTTPClient(ExecutorConfig{
		ExecutorHTTPTimeout:     15 * time.Second,
		ExecutorIdleConnTimeout: 20 * time.Second,
	})
	require.Equal(t,
		15*time.
			Second, client.
			Timeout,
	)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t,
		ok)
	require.Equal(t,
		20*time.
			Second, transport.
			IdleConnTimeout,
	)

}
