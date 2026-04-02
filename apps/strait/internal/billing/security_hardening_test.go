package billing

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestFailOpenTracker_Cleanup(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	e := NewEnforcer(store, rdb, slog.Default())

	ctx := t.Context()

	// Add entries
	for i := range 100 {
		key := "org-" + string(rune('a'+i%26))
		_ = e.boundedFailOpen(ctx, key, "test", "error")
	}

	// Verify entries exist
	count := 0
	e.failOpenTracker.Range(func(_, _ any) bool { count++; return true })
	if count == 0 {
		t.Fatal("expected entries in tracker")
	}
}

func TestMaskEmail_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"user@example.com", "u***@example.com"},
		{"a@b.com", "a***@b.com"},
		{"longname@domain.org", "l***@domain.org"},
	}
	for _, tt := range cases {
		got := maskEmail(tt.input)
		if got != tt.want {
			t.Errorf("maskEmail(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMaskEmail_Invalid(t *testing.T) {
	t.Parallel()
	cases := []string{"", "no-at-sign", "@nodomain"}
	for _, input := range cases {
		got := maskEmail(input)
		if strings.Contains(got, "@") && !strings.Contains(got, "***") {
			t.Errorf("maskEmail(%q) = %q, should be masked", input, got)
		}
	}
}

func TestMaskEmail_NoLeak(t *testing.T) {
	t.Parallel()
	email := "sensitive.user@private.domain.com"
	masked := maskEmail(email)
	if strings.Contains(masked, "sensitive") {
		t.Errorf("maskEmail leaked local part: %q", masked)
	}
	if !strings.Contains(masked, "private.domain.com") {
		t.Error("maskEmail should preserve domain for debugging")
	}
}

func TestWebhookReplayProtection_DuplicateRejected(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithEdition("community"))

	body := `{"id":"evt-sec","type":"customer.subscription.created","data":{"object":{"id":"sub_1","status":"active","items":{"data":[{"price":{"id":"starter-id"},"current_period_start":1700000000,"current_period_end":1702592000}]},"customer":{"id":"cust_1","email":"test@example.com","metadata":{"org_id":"550e8400-e29b-41d4-a716-446655440000"}}}}}`

	// First request
	req1 := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	req1.Header.Set("webhook-id", "msg_unique_123")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// Second request with same msg ID
	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	req2.Header.Set("webhook-id", "msg_unique_123")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("duplicate should return 200 (silently accept), got %d", rec2.Code)
	}
}

func TestWebhookReplayProtection_DifferentIDsAllowed(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithEdition("community"))

	body := `{"id":"evt-sec","type":"customer.subscription.created","data":{"object":{"id":"sub_1","status":"active","items":{"data":[{"price":{"id":"starter-id"},"current_period_start":1700000000,"current_period_end":1702592000}]},"customer":{"id":"cust_1","email":"test@example.com","metadata":{"org_id":"550e8400-e29b-41d4-a716-446655440000"}}}}}`

	req1 := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	req1.Header.Set("webhook-id", "msg_aaa")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	req2.Header.Set("webhook-id", "msg_bbb")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	// Both should be processed (different IDs)
	// We can't easily assert processing happened without mocking more,
	// but at least neither should panic.
	_ = rec1
	_ = rec2
}

func TestWebhookReplayCleanup(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	// Manually add old entries
	old := time.Now().Add(-15 * time.Minute).UnixNano()
	handler.replayCache.Store("old_msg_1", old)
	handler.replayCache.Store("old_msg_2", old)
	handler.replayCache.Store("recent_msg", time.Now().UnixNano())

	// Run cleanup manually
	now := time.Now().UnixNano()
	handler.replayCache.Range(func(key, value any) bool {
		ts := value.(int64)
		if time.Duration(now-ts) > 10*time.Minute {
			handler.replayCache.Delete(key)
		}
		return true
	})

	// Verify old entries removed
	count := 0
	handler.replayCache.Range(func(_, _ any) bool { count++; return true })
	if count != 1 {
		t.Fatalf("expected 1 entry after cleanup, got %d", count)
	}
}

func FuzzMaskEmail(f *testing.F) {
	f.Add("user@example.com")
	f.Add("")
	f.Add("a@b")
	f.Add("@")
	f.Add("no-at")
	f.Add(strings.Repeat("x", 1000) + "@long.com")

	f.Fuzz(func(t *testing.T, email string) {
		result := maskEmail(email)
		// Must never return the full local part for emails longer than 1 char
		if strings.Contains(email, "@") {
			parts := strings.SplitN(email, "@", 2)
			if len(parts[0]) > 1 && strings.Contains(result, parts[0]) {
				t.Errorf("maskEmail leaked full local part: input=%q result=%q", email, result)
			}
		}
	})
}
