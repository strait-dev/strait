package logdrain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewService_ClientTimeout(t *testing.T) {
	t.Parallel()
	svc := NewService()
	assert.Equal(t, 10*
		time.Second, svc.
		client.
		Timeout,
	)

}

func TestDrainRunEvents_DefaultClientBlocksDNSRebindingAtSendTime(t *testing.T) {
	previousTransport := newServiceTransport
	newServiceTransport = httputil.NewExternalTransport
	t.Cleanup(func() {
		newServiceTransport = previousTransport
	})

	var lookups atomic.Int32
	restore := httputil.SetLookupHostForTest(func(host string) ([]string, error) {
		if host != "rebind.test" {
			return nil, fmt.Errorf("unexpected host lookup: %s", host)
		}
		lookups.Add(1)
		return []string{"127.0.0.1"}, nil
	})
	t.Cleanup(restore)

	svc := NewService()
	err := svc.DrainRunEvents(context.Background(), &domain.LogDrain{
		EndpointURL: "http://rebind.test/drain",
		AuthType:    "none",
	}, []domain.RunEvent{{ID: "evt-1", RunID: "run-1"}})
	require.Error(t, err)
	require.False(t, !strings.Contains(
		err.Error(),
		"private address",
	) && !strings.Contains(err.
		Error(), "resolves to private",
	))
	require.NotEqual(t,
		0, lookups.Load())

}

func TestDrainRunEvents(t *testing.T) {
	makeEvents := func() []domain.RunEvent {
		return []domain.RunEvent{
			{ID: "evt-1", RunID: "run-1", Message: "step started"},
			{ID: "evt-2", RunID: "run-1", Message: "step completed"},
		}
	}

	tests := []struct {
		name       string
		drain      domain.LogDrain
		status     int
		checkReq   func(t *testing.T, r *http.Request, body []byte)
		wantErr    bool
		errContain string
	}{
		{
			name:   "successful drain delivers events",
			status: http.StatusOK,
			drain: domain.LogDrain{
				AuthType: "",
			},
			checkReq: func(t *testing.T, r *http.Request, body []byte) {
				t.Helper()
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				var got []domain.RunEvent
				require.NoError(t,
					json.Unmarshal(body,
						&got,
					))
				require.Len(t, got,
					2)
				assert.False(t, got[0].ID != "evt-1" ||
					got[1].ID !=
						"evt-2",
				)

			},
		},
		{
			name:   "bearer auth sets Authorization header",
			status: http.StatusOK,
			drain: domain.LogDrain{
				AuthType:   "bearer",
				AuthConfig: map[string]string{"token": "tk_secret123"},
			},
			checkReq: func(t *testing.T, r *http.Request, _ []byte) {
				t.Helper()
				want := "Bearer tk_secret123"
				assert.Equal(t, want, r.Header.Get("Authorization"))
			},
		},
		{
			name:   "basic auth sets credentials",
			status: http.StatusOK,
			drain: domain.LogDrain{
				AuthType:   "basic",
				AuthConfig: map[string]string{"username": "user", "password": "pass"},
			},
			checkReq: func(t *testing.T, r *http.Request, _ []byte) {
				t.Helper()
				u, p, ok := r.BasicAuth()
				require.True(t, ok)
				assert.False(t, u !=
					"user" || p !=
					"pass",
				)

			},
		},
		{
			name:   "header auth sets custom headers",
			status: http.StatusOK,
			drain: domain.LogDrain{
				AuthType: "header",
				AuthConfig: map[string]string{
					"X-Api-Key":    "key-abc",
					"X-Api-Secret": "secret-xyz",
				},
			},
			checkReq: func(t *testing.T, r *http.Request, _ []byte) {
				t.Helper()
				assert.Equal(t, "key-abc", r.Header.Get("X-Api-Key"))
				assert.Equal(t, "secret-xyz", r.Header.Get("X-Api-Secret"))
			},
		},
		{
			name:       "4xx response returns error",
			status:     http.StatusBadRequest,
			drain:      domain.LogDrain{AuthType: ""},
			wantErr:    true,
			errContain: "400",
		},
		{
			name:       "5xx response returns error",
			status:     http.StatusBadGateway,
			drain:      domain.LogDrain{AuthType: ""},
			wantErr:    true,
			errContain: "502",
		},
		{
			name:   "header auth blocks protected headers",
			status: http.StatusOK,
			drain: domain.LogDrain{
				AuthType: "header",
				AuthConfig: map[string]string{
					"X-Custom":          "allowed",
					"Host":              "evil.com",
					"Content-Type":      "text/plain",
					"Content-Length":    "999",
					"Transfer-Encoding": "chunked",
					"Connection":        "keep-alive",
				},
			},
			checkReq: func(t *testing.T, r *http.Request, _ []byte) {
				t.Helper()
				assert.Equal(t, "allowed", r.Header.Get("X-Custom"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.NotEqual(t,
					"evil.com", r.Host,
				)

			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var capturedReq *http.Request
			var capturedBody []byte

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedReq = r.Clone(r.Context())
				capturedBody, _ = io.ReadAll(r.Body)
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			drain := tt.drain
			drain.EndpointURL = srv.URL

			svc := NewService()
			err := svc.DrainRunEvents(context.Background(), &drain, makeEvents())

			if tt.wantErr {
				require.Error(t, err)
				assert.False(t, tt.
					errContain != "" &&
					!strings.Contains(err.
						Error(), tt.errContain,
					))

				return
			}
			require.NoError(t,
				err)

			if tt.checkReq != nil {
				tt.checkReq(t, capturedReq, capturedBody)
			}
		})
	}
}
