package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestDeviceCode_Success(t *testing.T) {
	t.Parallel()

	want := DeviceCodeResponse{
		DeviceCode:      "dev-abc123",
		UserCode:        "ABCD-1234",
		VerificationURL: "https://app.example.com/device",
		ExpiresIn:       900,
		Interval:        5,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/v1/cli/auth/device-code" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want) //nolint:errcheck
	}))
	defer srv.Close()

	c, err := New(srv.URL, "", 10*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	got, err := c.RequestDeviceCode(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.DeviceCode != want.DeviceCode {
		t.Errorf("device code: got %q, want %q", got.DeviceCode, want.DeviceCode)
	}
	if got.UserCode != want.UserCode {
		t.Errorf("user code: got %q, want %q", got.UserCode, want.UserCode)
	}
	if got.VerificationURL != want.VerificationURL {
		t.Errorf("verification URL: got %q, want %q", got.VerificationURL, want.VerificationURL)
	}
	if got.ExpiresIn != want.ExpiresIn {
		t.Errorf("expires in: got %d, want %d", got.ExpiresIn, want.ExpiresIn)
	}
	if got.Interval != want.Interval {
		t.Errorf("interval: got %d, want %d", got.Interval, want.Interval)
	}
}

func TestRequestDeviceCode_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c, err := New(srv.URL, "", 10*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = c.RequestDeviceCode(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if want := "device code request failed (500)"; !containsStr(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}

func TestPollDeviceToken_ReturnsOnApproval(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"}) //nolint:errcheck
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DeviceTokenResponse{ //nolint:errcheck
			APIKey:    "sk_test_approved",
			ProjectID: "proj_123",
			Scopes:    []string{"jobs:read", "jobs:write"},
		})
	}))
	defer srv.Close()

	c, err := New(srv.URL, "", 10*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	// Use a very short interval for test speed.
	got, err := c.PollDeviceToken(context.Background(), "dev-abc", 1, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.APIKey != "sk_test_approved" {
		t.Errorf("api key: got %q, want %q", got.APIKey, "sk_test_approved")
	}
	if got.ProjectID != "proj_123" {
		t.Errorf("project id: got %q, want %q", got.ProjectID, "proj_123")
	}
	if len(got.Scopes) != 2 {
		t.Errorf("scopes length: got %d, want 2", len(got.Scopes))
	}
	if total := calls.Load(); total != 3 {
		t.Errorf("expected 3 calls, got %d", total)
	}
}

func TestPollDeviceToken_StopsOnExpiry(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "expired_token"}) //nolint:errcheck
	}))
	defer srv.Close()

	c, err := New(srv.URL, "", 10*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = c.PollDeviceToken(context.Background(), "dev-expired", 1, 30)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if want := "device code expired"; !containsStr(err.Error(), want) {
		t.Errorf("error %q should contain %q", err.Error(), want)
	}
}

func TestPollDeviceToken_RespectsContext(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"}) //nolint:errcheck
	}))
	defer srv.Close()

	c, err := New(srv.URL, "", 10*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = c.PollDeviceToken(ctx, "dev-cancel", 1, 300)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsStr(err.Error(), "context") {
		t.Errorf("expected context error, got: %v", err)
	}
}

func TestPollDeviceToken_HandlesSlowPoll(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			// First call returns 429 to trigger backoff.
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		// Second call succeeds.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DeviceTokenResponse{ //nolint:errcheck
			APIKey:    "sk_test_backoff",
			ProjectID: "proj_456",
		})
	}))
	defer srv.Close()

	c, err := New(srv.URL, "", 10*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	got, err := c.PollDeviceToken(context.Background(), "dev-slow", 1, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.APIKey != "sk_test_backoff" {
		t.Errorf("api key: got %q, want %q", got.APIKey, "sk_test_backoff")
	}
	if total := calls.Load(); total != 2 {
		t.Errorf("expected 2 calls, got %d", total)
	}
}

func TestPollDeviceToken_Pending(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"}) //nolint:errcheck
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DeviceTokenResponse{ //nolint:errcheck
			APIKey:    "sk_test_pending",
			ProjectID: "proj_789",
			Scopes:    []string{"*"},
		})
	}))
	defer srv.Close()

	c, err := New(srv.URL, "", 10*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	got, err := c.PollDeviceToken(context.Background(), "dev-pending", 1, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.APIKey != "sk_test_pending" {
		t.Errorf("api key: got %q, want %q", got.APIKey, "sk_test_pending")
	}
	if total := calls.Load(); total != 3 {
		t.Errorf("expected 3 poll attempts, got %d", total)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
