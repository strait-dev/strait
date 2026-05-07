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
)

func TestNewService_ClientTimeout(t *testing.T) {
	t.Parallel()
	svc := NewService()
	if svc.client.Timeout != 10*time.Second {
		t.Errorf("client.Timeout = %v, want 10s", svc.client.Timeout)
	}
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
	if err == nil {
		t.Fatal("expected DNS rebinding attempt to be blocked")
	}
	if !strings.Contains(err.Error(), "private address") && !strings.Contains(err.Error(), "resolves to private") {
		t.Fatalf("expected private-address rejection, got %v", err)
	}
	if lookups.Load() == 0 {
		t.Fatal("expected SSRF validation to resolve the drain host")
	}
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
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", ct)
				}
				var got []domain.RunEvent
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("unmarshal body: %v", err)
				}
				if len(got) != 2 {
					t.Fatalf("got %d events, want 2", len(got))
				}
				if got[0].ID != "evt-1" || got[1].ID != "evt-2" {
					t.Errorf("unexpected event IDs: %v", got)
				}
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
				if got := r.Header.Get("Authorization"); got != want {
					t.Errorf("Authorization = %q, want %q", got, want)
				}
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
				if !ok {
					t.Fatal("expected basic auth credentials")
				}
				if u != "user" || p != "pass" {
					t.Errorf("basic auth = (%q, %q), want (user, pass)", u, p)
				}
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
				if got := r.Header.Get("X-Api-Key"); got != "key-abc" {
					t.Errorf("X-Api-Key = %q, want key-abc", got)
				}
				if got := r.Header.Get("X-Api-Secret"); got != "secret-xyz" {
					t.Errorf("X-Api-Secret = %q, want secret-xyz", got)
				}
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
				if got := r.Header.Get("X-Custom"); got != "allowed" {
					t.Errorf("X-Custom = %q, want allowed", got)
				}
				if got := r.Header.Get("Content-Type"); got != "application/json" {
					t.Errorf("Content-Type = %q, want application/json (should not be overridden)", got)
				}
				if r.Host == "evil.com" {
					t.Error("Host header was overridden to evil.com — should be blocked")
				}
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
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error %q does not contain %q", err, tt.errContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkReq != nil {
				tt.checkReq(t, capturedReq, capturedBody)
			}
		})
	}
}
