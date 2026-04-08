package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/cli"
)

func newTestClient(srv *httptest.Server) *cli.Client {
	return cli.NewClient(&cli.Profile{
		APIURL: srv.URL,
		APIKey: "test_key",
	})
}

func TestPollForToken_ApprovesAfterTwoPolls(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/cli/auth/token" {
			http.NotFound(w, r)
			return
		}
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"api_key":    "sk_real",
			"project_id": "proj_abc",
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	tok, err := cli.PollForToken(context.Background(), c, "dc_test", 10*time.Millisecond, time.Now().Add(5*time.Second))
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if tok.APIKey != "sk_real" {
		t.Errorf("api_key: got %q want %q", tok.APIKey, "sk_real")
	}
	if calls < 3 {
		t.Errorf("expected at least 3 calls, got %d", calls)
	}
}

func TestPollForToken_ExpiresGracefully(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "expired_token"})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := cli.PollForToken(context.Background(), c, "dc_expired", 10*time.Millisecond, time.Now().Add(5*time.Second))
	if !errors.Is(err, cli.ErrExpiredToken) {
		t.Errorf("expected ErrExpiredToken, got %v", err)
	}
}

func TestPollForToken_AlreadyExchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "token_already_exchanged"})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := cli.PollForToken(context.Background(), c, "dc_used", 10*time.Millisecond, time.Now().Add(5*time.Second))
	if !errors.Is(err, cli.ErrAlreadyExchanged) {
		t.Errorf("expected ErrAlreadyExchanged, got %v", err)
	}
}

func TestPollForToken_DeadlineExceededByWallClock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	// Deadline already past — should return immediately on first tick.
	_, err := cli.PollForToken(context.Background(), c, "dc_test", 10*time.Millisecond, time.Now().Add(-1*time.Second))
	if !errors.Is(err, cli.ErrExpiredToken) {
		t.Errorf("expected ErrExpiredToken for past deadline, got %v", err)
	}
}

func TestAPIError_UnmarshalsFromHuma(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]string{{"message": "source_hash is required"}},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	err := c.Do(context.Background(), "POST", "/v1/anything", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}

	var apiErr *cli.APIError
	if ok := isAPIError(err, &apiErr); !ok {
		t.Fatalf("expected *cli.APIError, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusUnprocessableEntity {
		t.Errorf("status: got %d want %d", apiErr.Status, http.StatusUnprocessableEntity)
	}
	if apiErr.Message != "source_hash is required" {
		t.Errorf("message: got %q want %q", apiErr.Message, "source_hash is required")
	}
}

// isAPIError extracts a *cli.APIError from err via errors.As.
func isAPIError(err error, target **cli.APIError) bool {
	return errors.As(err, target)
}
