package api

import (
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

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
