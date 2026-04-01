package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
)

// Tests: Fix 1: safeIDPrefix does not panic on short/empty strings.

func TestSafeIDPrefixWithEmptyString(t *testing.T) {
	t.Parallel()
	got := safeIDPrefix("")
	if got != "" {
		t.Fatalf("safeIDPrefix(\"\") = %q, want \"\"", got)
	}
}

func TestSafeIDPrefixWithShortString(t *testing.T) {
	t.Parallel()
	got := safeIDPrefix("abc")
	if got != "abc" {
		t.Fatalf("safeIDPrefix(\"abc\") = %q, want \"abc\"", got)
	}
}

func TestSafeIDPrefixWithExact8(t *testing.T) {
	t.Parallel()
	got := safeIDPrefix("12345678")
	if got != "12345678" {
		t.Fatalf("safeIDPrefix(\"12345678\") = %q, want \"12345678\"", got)
	}
}

func TestSafeIDPrefixWithLongString(t *testing.T) {
	t.Parallel()
	got := safeIDPrefix("1234567890abcdef")
	if got != "12345678" {
		t.Fatalf("safeIDPrefix(long) = %q, want \"12345678\"", got)
	}
}

// Tests: Fix 1: GenerateRecommendations with nil agent.

func TestGenerateRecommendationsNilAgent(t *testing.T) {
	t.Parallel()
	_, err := GenerateRecommendations(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for nil agent")
	}
}

// Tests: Fix 1: GenerateRecommendations with empty agent ID.

type mockCostStore struct {
	runs []domain.JobRun
	err  error
}

func (m *mockCostStore) ListRunsByJob(_ context.Context, _ string, limit, _ int) ([]domain.JobRun, error) {
	if m.err != nil {
		return nil, m.err
	}
	if limit > 0 && limit < len(m.runs) {
		return m.runs[:limit], nil
	}
	return m.runs, nil
}

func TestGenerateRecommendationsShortAgentID(t *testing.T) {
	t.Parallel()
	runs := make([]domain.JobRun, 25)
	for i := range runs {
		runs[i] = domain.JobRun{Status: domain.StatusCompleted}
	}
	store := &mockCostStore{runs: runs}
	agent := &domain.Agent{ID: "ab", Model: "gpt-5.4", Config: json.RawMessage(`{}`)}

	recs, err := GenerateRecommendations(context.Background(), store, agent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not panic and should produce recommendations.
	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation")
	}
	// Verify the ID uses the safe prefix.
	for _, rec := range recs {
		if len(rec.ID) == 0 {
			t.Fatal("recommendation ID is empty")
		}
	}
}

// Tests: Fix 5: Messaging with empty SourceAgentID.

func TestSendMessageEmptySourceRejected(t *testing.T) {
	t.Parallel()
	store := newMockStore("a", "b")
	svc := NewAgentMessageService(store)

	_, err := svc.Send(context.Background(), SendRequest{
		ProjectID:     "proj-1",
		SourceAgentID: "",
		TargetAgentID: "b",
	})
	if err == nil {
		t.Fatal("expected error for empty SourceAgentID")
	}
}

func TestSendMessageEmptyTargetRejected(t *testing.T) {
	t.Parallel()
	store := newMockStore("a")
	svc := NewAgentMessageService(store)

	_, err := svc.Send(context.Background(), SendRequest{
		ProjectID:     "proj-1",
		SourceAgentID: "a",
		TargetAgentID: "",
	})
	if err == nil {
		t.Fatal("expected error for empty TargetAgentID")
	}
}

func TestSendMessageEmptyProjectRejected(t *testing.T) {
	t.Parallel()
	store := newMockStore("a", "b")
	svc := NewAgentMessageService(store)

	_, err := svc.Send(context.Background(), SendRequest{
		ProjectID:     "",
		SourceAgentID: "a",
		TargetAgentID: "b",
	})
	if err == nil {
		t.Fatal("expected error for empty ProjectID")
	}
}

// Tests: Fix 9: Canary router with nil randFn does not panic.

func TestCanaryRouterNilRandFnDefaultsToSource(t *testing.T) {
	t.Parallel()
	router := &AgentCanaryRouter{randFn: nil}
	canary := &AgentCanaryDeployment{
		Status:             AgentCanaryStatusActive,
		SourceDeploymentID: "source",
		TargetDeploymentID: "target",
		TrafficPct:         50,
	}
	got := router.Route(canary)
	if got != "source" {
		t.Fatalf("Route() with nil randFn = %q, want source", got)
	}
}

// Tests: Fix 11: Messaging distinguishes store errors from not-found.

type failingMessageStore struct {
	mockMessageStore
}

func (m *failingMessageStore) GetAgent(_ context.Context, _ string) (*domain.Agent, error) {
	return nil, errors.New("database connection lost")
}

func TestSendMessageStoreErrorNotMasked(t *testing.T) {
	t.Parallel()
	store := &failingMessageStore{
		mockMessageStore: mockMessageStore{
			agents: map[string]*domain.Agent{},
		},
	}
	svc := NewAgentMessageService(store)

	_, err := svc.Send(context.Background(), SendRequest{
		ProjectID:     "proj-1",
		SourceAgentID: "a",
		TargetAgentID: "b",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// The error should NOT be ErrTargetNotFound since it's a store failure.
	if errors.Is(err, ErrTargetNotFound) {
		t.Fatal("store connection error should not be masked as ErrTargetNotFound")
	}
	if got := err.Error(); got == "" {
		t.Fatal("error message is empty")
	}
}

// Tests: Fix 7: ExtractWebhookURL with various inputs.

func TestExtractWebhookURLEmpty(t *testing.T) {
	t.Parallel()
	if got := ExtractWebhookURL(nil); got != "" {
		t.Fatalf("ExtractWebhookURL(nil) = %q", got)
	}
	if got := ExtractWebhookURL(json.RawMessage(`{}`)); got != "" {
		t.Fatalf("ExtractWebhookURL({}) = %q", got)
	}
	if got := ExtractWebhookURL(json.RawMessage(`invalid`)); got != "" {
		t.Fatalf("ExtractWebhookURL(invalid) = %q", got)
	}
}

func TestExtractWebhookURLValid(t *testing.T) {
	t.Parallel()
	got := ExtractWebhookURL(json.RawMessage(`{"webhook_url":"https://example.com/hook"}`))
	if got != "https://example.com/hook" {
		t.Fatalf("got %q, want https://example.com/hook", got)
	}
}

func TestExtractWebhookURLTrimsWhitespace(t *testing.T) {
	t.Parallel()
	got := ExtractWebhookURL(json.RawMessage(`{"webhook_url":"  https://example.com/hook  "}`))
	if got != "https://example.com/hook" {
		t.Fatalf("got %q, want trimmed URL", got)
	}
}

func TestIsSafeWebhookURLBlocksSSRF(t *testing.T) {
	t.Parallel()

	blocked := []struct {
		name string
		url  string
	}{
		{"cloud metadata IPv4", "https://169.254.169.254/latest/meta-data/"},
		{"google metadata", "https://metadata.google.internal/computeMetadata/v1/"},
		{"localhost", "https://localhost:8080/admin"},
		{"loopback", "https://127.0.0.1/admin"},
		{"zero addr", "https://0.0.0.0/admin"},
		{"dot local", "https://redis.local/"},
		{"dot internal", "https://db.internal/"},
		{"http scheme", "http://example.com/hook"},
		{"ftp scheme", "ftp://example.com/hook"},
		{"empty scheme", "//example.com/hook"},
		{"invalid url", "://badurl"},
	}

	for _, tt := range blocked {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if isSafeWebhookURL(tt.url) {
				t.Fatalf("isSafeWebhookURL(%q) should be false", tt.url)
			}
		})
	}

	allowed := []struct {
		name string
		url  string
	}{
		{"public https google", "https://www.google.com/webhooks"},
		{"public https github", "https://github.com/webhooks"},
	}

	for _, tt := range allowed {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !isSafeWebhookURL(tt.url) {
				t.Fatalf("isSafeWebhookURL(%q) should be true", tt.url)
			}
		})
	}
}

func TestExtractWebhookURLRejectsHTTP(t *testing.T) {
	t.Parallel()
	got := ExtractWebhookURL(json.RawMessage(`{"webhook_url":"http://example.com/hook"}`))
	if got != "" {
		t.Fatalf("http webhook should be rejected, got %q", got)
	}
}

func TestExtractWebhookURLRejectsInternalHosts(t *testing.T) {
	t.Parallel()
	got := ExtractWebhookURL(json.RawMessage(`{"webhook_url":"https://169.254.169.254/latest/"}`))
	if got != "" {
		t.Fatalf("metadata endpoint should be rejected, got %q", got)
	}
}

func TestIsSafeWebhookURLBlocksDNSRebinding(t *testing.T) {
	t.Parallel()
	// A real hostname that resolves to a loopback should be blocked.
	// localhost.localdomain typically resolves to 127.0.0.1.
	if isSafeWebhookURL("https://localhost.localdomain/hook") {
		t.Fatal("localhost.localdomain should be blocked by DNS resolution check")
	}
}

func TestIsSafeWebhookURLBlocksPrivateIPHostnames(t *testing.T) {
	t.Parallel()
	// A direct private IP in hostname should be blocked both by DNS resolution
	// and by the hostname blocklist.
	if isSafeWebhookURL("https://127.0.0.1/hook") {
		t.Fatal("127.0.0.1 should be blocked")
	}
}

func TestChainMessageLimitEnforced(t *testing.T) {
	t.Parallel()
	store := newMockStore("a", "b")
	svc := NewAgentMessageService(store)
	ctx := context.Background()

	// Pre-populate chain with maxMessagesPerChain messages.
	for i := range maxMessagesPerChain {
		store.messages = append(store.messages, domain.AgentMessage{
			SourceAgentID: "a",
			TargetAgentID: "b",
			ChainID:       "flood-chain",
			ChainDepth:    i + 1,
		})
	}

	// The next message should be rejected by message count limit.
	_, err := svc.Send(ctx, SendRequest{
		ProjectID:     "proj-1",
		SourceAgentID: "a",
		TargetAgentID: "b",
		ChainID:       "flood-chain",
		ChainDepth:    maxChainDepth, // within depth limit but over message count
	})
	if !errors.Is(err, ErrChainMessageLimit) {
		t.Fatalf("expected ErrChainMessageLimit, got %v", err)
	}
}

func TestBeginningOfMonth(t *testing.T) {
	t.Parallel()
	now, _ := time.Parse(time.RFC3339, "2026-03-15T14:30:00Z")
	got := beginningOfMonth(now)
	want, _ := time.Parse(time.RFC3339, "2026-03-01T00:00:00Z")
	if !got.Equal(want) {
		t.Fatalf("beginningOfMonth(%v) = %v, want %v", now, got, want)
	}
}

func TestNormalizePayload_Nil(t *testing.T) {
	t.Parallel()
	got := normalizePayload(nil)
	if string(got) != "{}" {
		t.Fatalf("normalizePayload(nil) = %q, want {}", string(got))
	}
}

func TestNormalizePayload_Empty(t *testing.T) {
	t.Parallel()
	got := normalizePayload(json.RawMessage{})
	if string(got) != "{}" {
		t.Fatalf("normalizePayload(empty) = %q, want {}", string(got))
	}
}

func TestNormalizePayload_Preserves(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`{"key":"value"}`)
	got := normalizePayload(input)
	if string(got) != `{"key":"value"}` {
		t.Fatalf("normalizePayload(valid) = %q", string(got))
	}
}
