package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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
}

func (m *mockCostStore) ListRunsByJob(_ context.Context, _ string, _, _ int) ([]domain.JobRun, error) {
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

// Tests: Fix 7: extractWebhookURL with various inputs.

func TestExtractWebhookURLEmpty(t *testing.T) {
	t.Parallel()
	if got := extractWebhookURL(nil); got != "" {
		t.Fatalf("extractWebhookURL(nil) = %q", got)
	}
	if got := extractWebhookURL(json.RawMessage(`{}`)); got != "" {
		t.Fatalf("extractWebhookURL({}) = %q", got)
	}
	if got := extractWebhookURL(json.RawMessage(`invalid`)); got != "" {
		t.Fatalf("extractWebhookURL(invalid) = %q", got)
	}
}

func TestExtractWebhookURLValid(t *testing.T) {
	t.Parallel()
	got := extractWebhookURL(json.RawMessage(`{"webhook_url":"https://example.com/hook"}`))
	if got != "https://example.com/hook" {
		t.Fatalf("got %q, want https://example.com/hook", got)
	}
}

func TestExtractWebhookURLTrimsWhitespace(t *testing.T) {
	t.Parallel()
	got := extractWebhookURL(json.RawMessage(`{"webhook_url":"  https://example.com/hook  "}`))
	if got != "https://example.com/hook" {
		t.Fatalf("got %q, want trimmed URL", got)
	}
}
