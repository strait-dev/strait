package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

type notifyStoreAdapter struct {
	store.NotifyStore
	getActiveEscalationStateByStepRunFunc       func(ctx context.Context, projectID, stepRunID string) (*domain.EscalationState, error)
	acknowledgeEscalationByStepRunFunc          func(ctx context.Context, stepRunID, acknowledgedBy string, acknowledgedAt time.Time) error
	completeActiveEscalationByStepRunStatusFunc func(ctx context.Context, stepRunID, status string) error
}

func (m *notifyStoreAdapter) GetActiveEscalationStateByStepRun(ctx context.Context, projectID, stepRunID string) (*domain.EscalationState, error) {
	if m.getActiveEscalationStateByStepRunFunc == nil {
		return nil, store.ErrEscalationStateNotFound
	}
	return m.getActiveEscalationStateByStepRunFunc(ctx, projectID, stepRunID)
}

func (m *notifyStoreAdapter) AcknowledgeActiveEscalationStateByStepRun(ctx context.Context, stepRunID, acknowledgedBy string, acknowledgedAt time.Time) error {
	if m.acknowledgeEscalationByStepRunFunc == nil {
		return nil
	}
	return m.acknowledgeEscalationByStepRunFunc(ctx, stepRunID, acknowledgedBy, acknowledgedAt)
}

func (m *notifyStoreAdapter) CompleteActiveEscalationStateByStepRun(ctx context.Context, stepRunID, status string) error {
	if m.completeActiveEscalationByStepRunStatusFunc == nil {
		return nil
	}
	return m.completeActiveEscalationByStepRunStatusFunc(ctx, stepRunID, status)
}

type notifyAPIStore struct {
	*APIStoreMock
	store.NotifyStore
}

func TestNotifySubscriberTokenRoundTrip(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	token, err := srv.createNotifySubscriberToken("sub_123", "proj_123", "tenant_123", time.Hour)
	if err != nil {
		t.Fatalf("createNotifySubscriberToken() error = %v", err)
	}

	claims, err := srv.parseNotifySubscriberToken(token)
	if err != nil {
		t.Fatalf("parseNotifySubscriberToken() error = %v", err)
	}
	if claims.SubscriberID != "sub_123" {
		t.Fatalf("SubscriberID = %q, want sub_123", claims.SubscriberID)
	}
	if claims.ProjectID != "proj_123" {
		t.Fatalf("ProjectID = %q, want proj_123", claims.ProjectID)
	}
	if claims.TenantID != "tenant_123" {
		t.Fatalf("TenantID = %q, want tenant_123", claims.TenantID)
	}
}

func TestResolveNotifySchedule(t *testing.T) {
	t.Parallel()

	scheduled, err := resolveNotifySchedule(&NotifyScheduleInput{Delay: "5m"})
	if err != nil {
		t.Fatalf("resolveNotifySchedule(delay) error = %v", err)
	}
	if scheduled == nil {
		t.Fatal("resolveNotifySchedule(delay) returned nil")
	}

	at := "2026-04-04T09:00:00Z"
	scheduled, err = resolveNotifySchedule(&NotifyScheduleInput{At: at})
	if err != nil {
		t.Fatalf("resolveNotifySchedule(at) error = %v", err)
	}
	if scheduled == nil || scheduled.Format(time.RFC3339) != at {
		t.Fatalf("resolveNotifySchedule(at) = %v, want %s", scheduled, at)
	}

	if _, err := resolveNotifySchedule(&NotifyScheduleInput{Delay: "bad"}); err == nil {
		t.Fatal("resolveNotifySchedule(invalid) expected error")
	}
}

func TestResolveNotifyDigestWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 5, 13, 34, 10, 0, time.UTC)

	hourly, ok := resolveNotifyDigestWindow(notifyDigestPolicyHourly, now)
	if !ok {
		t.Fatal("resolveNotifyDigestWindow(hourly) ok = false, want true")
	}
	if hourly.Format(time.RFC3339) != "2026-04-05T14:00:00Z" {
		t.Fatalf("hourly window = %s, want 2026-04-05T14:00:00Z", hourly.Format(time.RFC3339))
	}

	daily, ok := resolveNotifyDigestWindow(notifyDigestPolicyDaily, now)
	if !ok {
		t.Fatal("resolveNotifyDigestWindow(daily) ok = false, want true")
	}
	if daily.Format(time.RFC3339) != "2026-04-06T00:00:00Z" {
		t.Fatalf("daily window = %s, want 2026-04-06T00:00:00Z", daily.Format(time.RFC3339))
	}

	if _, ok := resolveNotifyDigestWindow("weekly", now); ok {
		t.Fatal("resolveNotifyDigestWindow(weekly) ok = true, want false")
	}
}

func TestBuildNotifyRenderContext(t *testing.T) {
	t.Parallel()

	sub := &domain.NotifySubscriber{
		ID:         "sub_1",
		ExternalID: "user_1",
		Email:      "alice@example.com",
		Locale:     "en",
		Attributes: []byte(`{"name":"Alice","plan":"pro"}`),
	}
	payload := map[string]any{"job": map[string]any{"name": "Export"}}
	system := map[string]any{"preferences_url": "https://example.com/preferences"}

	ctx := buildNotifyRenderContext(payload, sub, system)
	if _, ok := ctx["job"]; !ok {
		t.Fatal("expected top-level payload key in context")
	}
	subscriber := ctx["subscriber"].(map[string]any)
	if subscriber["name"] != "Alice" {
		t.Fatalf("subscriber.name = %v, want Alice", subscriber["name"])
	}
	if ctx["preferences_url"] != "https://example.com/preferences" {
		t.Fatalf("preferences_url = %v, want https://example.com/preferences", ctx["preferences_url"])
	}

	encoded, err := json.Marshal(ctx)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("context should marshal: err=%v", err)
	}
}

func TestGetNotifyEscalationByStepRun(t *testing.T) {
	t.Parallel()

	ns := &notifyStoreAdapter{
		getActiveEscalationStateByStepRunFunc: func(_ context.Context, projectID, stepRunID string) (*domain.EscalationState, error) {
			if projectID != "proj_1" || stepRunID != "step_1" {
				t.Fatalf("GetActiveEscalationStateByStepRun args = (%q, %q), want (proj_1, step_1)", projectID, stepRunID)
			}
			return &domain.EscalationState{ID: "esc_1", ProjectID: projectID, StepRunID: stepRunID, Status: domain.NotifyEscalationStatusActive}, nil
		},
	}
	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/notify/escalations/step-runs/step_1", "", "proj_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["id"] != "esc_1" {
		t.Fatalf("id = %v, want esc_1", body["id"])
	}
}

func TestAcknowledgeNotifyEscalationByStepRun(t *testing.T) {
	t.Parallel()

	called := false
	ns := &notifyStoreAdapter{
		getActiveEscalationStateByStepRunFunc: func(_ context.Context, _, _ string) (*domain.EscalationState, error) {
			return &domain.EscalationState{ID: "esc_1", StepRunID: "step_1", Status: domain.NotifyEscalationStatusActive}, nil
		},
		acknowledgeEscalationByStepRunFunc: func(_ context.Context, stepRunID, acknowledgedBy string, acknowledgedAt time.Time) error {
			called = true
			if stepRunID != "step_1" {
				t.Fatalf("stepRunID = %q, want step_1", stepRunID)
			}
			if acknowledgedBy != "user_1" {
				t.Fatalf("acknowledgedBy = %q, want user_1", acknowledgedBy)
			}
			if acknowledgedAt.IsZero() {
				t.Fatal("acknowledgedAt is zero")
			}
			return nil
		},
	}
	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/notify/escalations/step-runs/step_1/acknowledge", "{}", "proj_1")
	r.Header.Set("X-Actor-Id", "user_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !called {
		t.Fatal("expected AcknowledgeActiveEscalationStateByStepRun to be called")
	}
}

func TestAcknowledgeNotifyEscalationByStepRun_NotFound(t *testing.T) {
	t.Parallel()

	ns := &notifyStoreAdapter{
		getActiveEscalationStateByStepRunFunc: func(_ context.Context, _, _ string) (*domain.EscalationState, error) {
			return nil, store.ErrEscalationStateNotFound
		},
	}
	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/notify/escalations/step-runs/step_1/acknowledge", "{}", "proj_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestCompleteNotifyEscalationByStepRun_Validation(t *testing.T) {
	t.Parallel()

	ns := &notifyStoreAdapter{}
	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/notify/escalations/step-runs/step_1/complete", `{"status":"acknowledged"}`, "proj_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCompleteNotifyEscalationByStepRun(t *testing.T) {
	t.Parallel()

	called := false
	ns := &notifyStoreAdapter{
		getActiveEscalationStateByStepRunFunc: func(_ context.Context, _, _ string) (*domain.EscalationState, error) {
			return &domain.EscalationState{ID: "esc_1", StepRunID: "step_1", Status: domain.NotifyEscalationStatusActive}, nil
		},
		completeActiveEscalationByStepRunStatusFunc: func(_ context.Context, stepRunID, status string) error {
			called = true
			if stepRunID != "step_1" {
				t.Fatalf("stepRunID = %q, want step_1", stepRunID)
			}
			if status != domain.NotifyEscalationStatusCompleted {
				t.Fatalf("status = %q, want %q", status, domain.NotifyEscalationStatusCompleted)
			}
			return nil
		},
	}
	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/notify/escalations/step-runs/step_1/complete", `{}`, "proj_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !called {
		t.Fatal("expected CompleteActiveEscalationStateByStepRun to be called")
	}
}
