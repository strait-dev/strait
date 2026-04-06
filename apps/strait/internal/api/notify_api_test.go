package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	upsertNotifyPolicyOverrideFunc              func(ctx context.Context, policy *domain.NotifyPolicyOverride) error
	getNotifyPolicyOverrideFunc                 func(ctx context.Context, id, projectID string) (*domain.NotifyPolicyOverride, error)
	listNotifyPolicyOverridesFunc               func(ctx context.Context, projectID string, scopeType *string) ([]domain.NotifyPolicyOverride, error)
	deleteNotifyPolicyOverrideFunc              func(ctx context.Context, id, projectID string) error
	getNotificationPreferenceFunc               func(ctx context.Context, recipientType, recipientID, scope string) (*domain.NotificationPreference, error)
	upsertNotificationPreferenceFunc            func(ctx context.Context, pref *domain.NotificationPreference) error
	listNotificationPreferencesFunc             func(ctx context.Context, recipientType, recipientID string) ([]domain.NotificationPreference, error)
	disableNotificationChannelPreferenceFunc    func(ctx context.Context, recipientType, recipientID, scope, channel string) error
	enableNotificationChannelPreferenceFunc     func(ctx context.Context, recipientType, recipientID, scope, channel string) error
	createNotifySuppressionEventFunc            func(ctx context.Context, event *domain.NotifySuppressionEvent) error
	listNotifySuppressionEventsFunc             func(ctx context.Context, projectID, recipientType, recipientID string, limit int, cursor *time.Time) ([]domain.NotifySuppressionEvent, error)
	getLatestNotifySuppressionEventFunc         func(ctx context.Context, projectID, recipientType, recipientID, scope, channel string) (*domain.NotifySuppressionEvent, error)
	recordNotifyProviderCallbackReceiptFunc     func(ctx context.Context, projectID, providerID, provider, callbackID, eventType, messageID, payloadHash string, expiresAt time.Time) (bool, error)
	deleteNotifyProviderCallbackReceiptFunc     func(ctx context.Context, projectID, providerID, callbackID string) error
	getNotificationMessageFunc                  func(ctx context.Context, id, projectID string) (*domain.NotificationMessage, error)
	getNotificationProviderFunc                 func(ctx context.Context, id, projectID string) (*domain.NotificationProvider, error)
	getNotifySubscriberFunc                     func(ctx context.Context, id, projectID string) (*domain.NotifySubscriber, error)
	updateNotificationMessageStatusFunc         func(ctx context.Context, id, projectID, fromStatus, toStatus string, fields map[string]any) error
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

func (m *notifyStoreAdapter) UpsertNotifyPolicyOverride(ctx context.Context, policy *domain.NotifyPolicyOverride) error {
	if m.upsertNotifyPolicyOverrideFunc == nil {
		return nil
	}
	return m.upsertNotifyPolicyOverrideFunc(ctx, policy)
}

func (m *notifyStoreAdapter) GetNotifyPolicyOverride(ctx context.Context, id, projectID string) (*domain.NotifyPolicyOverride, error) {
	if m.getNotifyPolicyOverrideFunc == nil {
		return nil, store.ErrNotifyPolicyNotFound
	}
	return m.getNotifyPolicyOverrideFunc(ctx, id, projectID)
}

func (m *notifyStoreAdapter) ListNotifyPolicyOverrides(ctx context.Context, projectID string, scopeType *string) ([]domain.NotifyPolicyOverride, error) {
	if m.listNotifyPolicyOverridesFunc == nil {
		return nil, nil
	}
	return m.listNotifyPolicyOverridesFunc(ctx, projectID, scopeType)
}

func (m *notifyStoreAdapter) DeleteNotifyPolicyOverride(ctx context.Context, id, projectID string) error {
	if m.deleteNotifyPolicyOverrideFunc == nil {
		return nil
	}
	return m.deleteNotifyPolicyOverrideFunc(ctx, id, projectID)
}

func (m *notifyStoreAdapter) GetNotificationPreference(ctx context.Context, recipientType, recipientID, scope string) (*domain.NotificationPreference, error) {
	if m.getNotificationPreferenceFunc == nil {
		return nil, store.ErrNotificationPreferenceNotFound
	}
	return m.getNotificationPreferenceFunc(ctx, recipientType, recipientID, scope)
}

func (m *notifyStoreAdapter) UpsertNotificationPreference(ctx context.Context, pref *domain.NotificationPreference) error {
	if m.upsertNotificationPreferenceFunc == nil {
		return nil
	}
	return m.upsertNotificationPreferenceFunc(ctx, pref)
}

func (m *notifyStoreAdapter) ListNotificationPreferences(ctx context.Context, recipientType, recipientID string) ([]domain.NotificationPreference, error) {
	if m.listNotificationPreferencesFunc == nil {
		return []domain.NotificationPreference{}, nil
	}
	return m.listNotificationPreferencesFunc(ctx, recipientType, recipientID)
}

func (m *notifyStoreAdapter) DisableNotificationChannelPreference(ctx context.Context, recipientType, recipientID, scope, channel string) error {
	if m.disableNotificationChannelPreferenceFunc == nil {
		return nil
	}
	return m.disableNotificationChannelPreferenceFunc(ctx, recipientType, recipientID, scope, channel)
}

func (m *notifyStoreAdapter) EnableNotificationChannelPreference(ctx context.Context, recipientType, recipientID, scope, channel string) error {
	if m.enableNotificationChannelPreferenceFunc == nil {
		return nil
	}
	return m.enableNotificationChannelPreferenceFunc(ctx, recipientType, recipientID, scope, channel)
}

func (m *notifyStoreAdapter) CreateNotifySuppressionEvent(ctx context.Context, event *domain.NotifySuppressionEvent) error {
	if m.createNotifySuppressionEventFunc == nil {
		return nil
	}
	return m.createNotifySuppressionEventFunc(ctx, event)
}

func (m *notifyStoreAdapter) ListNotifySuppressionEvents(ctx context.Context, projectID, recipientType, recipientID string, limit int, cursor *time.Time) ([]domain.NotifySuppressionEvent, error) {
	if m.listNotifySuppressionEventsFunc == nil {
		return []domain.NotifySuppressionEvent{}, nil
	}
	return m.listNotifySuppressionEventsFunc(ctx, projectID, recipientType, recipientID, limit, cursor)
}

func (m *notifyStoreAdapter) GetLatestNotifySuppressionEvent(ctx context.Context, projectID, recipientType, recipientID, scope, channel string) (*domain.NotifySuppressionEvent, error) {
	if m.getLatestNotifySuppressionEventFunc == nil {
		return nil, store.ErrNotifySuppressionEventNotFound
	}
	return m.getLatestNotifySuppressionEventFunc(ctx, projectID, recipientType, recipientID, scope, channel)
}

func (m *notifyStoreAdapter) RecordNotifyProviderCallbackReceipt(
	ctx context.Context,
	projectID, providerID, provider, callbackID, eventType, messageID, payloadHash string,
	expiresAt time.Time,
) (bool, error) {
	if m.recordNotifyProviderCallbackReceiptFunc == nil {
		return true, nil
	}
	return m.recordNotifyProviderCallbackReceiptFunc(ctx, projectID, providerID, provider, callbackID, eventType, messageID, payloadHash, expiresAt)
}

func (m *notifyStoreAdapter) DeleteNotifyProviderCallbackReceipt(ctx context.Context, projectID, providerID, callbackID string) error {
	if m.deleteNotifyProviderCallbackReceiptFunc == nil {
		return nil
	}
	return m.deleteNotifyProviderCallbackReceiptFunc(ctx, projectID, providerID, callbackID)
}

func (m *notifyStoreAdapter) GetNotificationMessage(ctx context.Context, id, projectID string) (*domain.NotificationMessage, error) {
	if m.getNotificationMessageFunc == nil {
		return nil, store.ErrNotificationMessageNotFound
	}
	return m.getNotificationMessageFunc(ctx, id, projectID)
}

func (m *notifyStoreAdapter) GetNotificationProvider(ctx context.Context, id, projectID string) (*domain.NotificationProvider, error) {
	if m.getNotificationProviderFunc == nil {
		return nil, store.ErrNotificationProviderNotFound
	}
	return m.getNotificationProviderFunc(ctx, id, projectID)
}

func (m *notifyStoreAdapter) GetNotifySubscriber(ctx context.Context, id, projectID string) (*domain.NotifySubscriber, error) {
	if m.getNotifySubscriberFunc == nil {
		return nil, store.ErrNotifySubscriberNotFound
	}
	return m.getNotifySubscriberFunc(ctx, id, projectID)
}

func (m *notifyStoreAdapter) UpdateNotificationMessageStatus(ctx context.Context, id, projectID, fromStatus, toStatus string, fields map[string]any) error {
	if m.updateNotificationMessageStatusFunc == nil {
		return nil
	}
	return m.updateNotificationMessageStatusFunc(ctx, id, projectID, fromStatus, toStatus, fields)
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

func TestNotifySubscriberToken_InvalidIssuerRejected(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	token, err := srv.createNotifySubscriberToken("sub_123", "proj_123", "tenant_123", time.Hour)
	if err != nil {
		t.Fatalf("createNotifySubscriberToken() error = %v", err)
	}

	srv.config.NotifySubscriberTokenIssuer = "unexpected-issuer"
	if _, err := srv.parseNotifySubscriberToken(token); err == nil {
		t.Fatal("parseNotifySubscriberToken() expected issuer validation error")
	}
}

func TestUpdateNotifyPreferencesScope_OmittedChannelPrefsPreservesSuppressionState(t *testing.T) {
	t.Parallel()

	upsertCalled := false
	ns := &notifyStoreAdapter{
		getNotificationPreferenceFunc: func(_ context.Context, recipientType, recipientID, scope string) (*domain.NotificationPreference, error) {
			return &domain.NotificationPreference{
				RecipientType:    recipientType,
				RecipientID:      recipientID,
				Scope:            scope,
				ChannelPrefs:     []byte(`{"email":true,"inbox":true}`),
				Timezone:         "UTC",
				DigestPolicy:     "immediate",
				CriticalOverride: true,
			}, nil
		},
		upsertNotificationPreferenceFunc: func(_ context.Context, pref *domain.NotificationPreference) error {
			upsertCalled = true
			if pref.ChannelPrefs != nil {
				t.Fatalf("ChannelPrefs should be nil when omitted, got %s", string(pref.ChannelPrefs))
			}
			if pref.Timezone != "Europe/Berlin" {
				t.Fatalf("Timezone = %q, want Europe/Berlin", pref.Timezone)
			}
			return nil
		},
		listNotificationPreferencesFunc: func(_ context.Context, _, _ string) ([]domain.NotificationPreference, error) {
			return []domain.NotificationPreference{}, nil
		},
	}

	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)

	token, err := srv.createNotifySubscriberToken("sub_1", "proj_1", "", time.Hour)
	if err != nil {
		t.Fatalf("createNotifySubscriberToken() error = %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/v1/preferences/global", strings.NewReader(`{"timezone":"Europe/Berlin"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !upsertCalled {
		t.Fatal("expected UpsertNotificationPreference call")
	}
}

func TestCreateNotifyUnsuppress(t *testing.T) {
	t.Parallel()

	enabledCalled := false
	eventCalled := false
	ns := &notifyStoreAdapter{
		getNotifySubscriberFunc: func(_ context.Context, id, projectID string) (*domain.NotifySubscriber, error) {
			return &domain.NotifySubscriber{ID: id, ProjectID: projectID, ExternalID: "user_1"}, nil
		},
		enableNotificationChannelPreferenceFunc: func(_ context.Context, recipientType, recipientID, scope, channel string) error {
			enabledCalled = true
			if recipientType != domain.NotifyRecipientTypeSubscriber || recipientID != "sub_1" || scope != "global" || channel != "email" {
				t.Fatalf("unexpected enable args type=%q id=%q scope=%q channel=%q", recipientType, recipientID, scope, channel)
			}
			return nil
		},
		createNotifySuppressionEventFunc: func(_ context.Context, event *domain.NotifySuppressionEvent) error {
			eventCalled = true
			if event.Action != domain.NotifySuppressionActionUnsuppressed {
				t.Fatalf("event.Action = %q, want %q", event.Action, domain.NotifySuppressionActionUnsuppressed)
			}
			if event.Source != domain.NotifySuppressionSourceAdminAPI {
				t.Fatalf("event.Source = %q, want %q", event.Source, domain.NotifySuppressionSourceAdminAPI)
			}
			if event.Reason != "manual_review" {
				t.Fatalf("event.Reason = %q, want manual_review", event.Reason)
			}
			return nil
		},
	}

	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/subscribers/sub_1/suppressions/unsuppress", `{"channel":"email","scope":"global","reason":"manual_review"}`, "proj_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !enabledCalled {
		t.Fatal("expected EnableNotificationChannelPreference call")
	}
	if !eventCalled {
		t.Fatal("expected CreateNotifySuppressionEvent call")
	}
}

func TestCreateNotifyUnsuppress_RequiresForceForProviderComplaint(t *testing.T) {
	t.Parallel()

	enableCalled := false
	ns := &notifyStoreAdapter{
		getNotifySubscriberFunc: func(_ context.Context, id, projectID string) (*domain.NotifySubscriber, error) {
			return &domain.NotifySubscriber{ID: id, ProjectID: projectID}, nil
		},
		getLatestNotifySuppressionEventFunc: func(_ context.Context, projectID, recipientType, recipientID, scope, channel string) (*domain.NotifySuppressionEvent, error) {
			return &domain.NotifySuppressionEvent{
				ID:            "evt_1",
				ProjectID:     projectID,
				RecipientType: recipientType,
				RecipientID:   recipientID,
				Scope:         scope,
				Channel:       channel,
				Action:        domain.NotifySuppressionActionSuppressed,
				Reason:        "provider_callback:email.complained",
				Source:        domain.NotifySuppressionSourceProviderCallback,
			}, nil
		},
		enableNotificationChannelPreferenceFunc: func(_ context.Context, _, _, _, _ string) error {
			enableCalled = true
			return nil
		},
	}

	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/subscribers/sub_1/suppressions/unsuppress", `{"channel":"email","scope":"global"}`, "proj_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	if enableCalled {
		t.Fatal("expected enable to be blocked by policy")
	}
}

func TestCreateNotifyUnsuppress_ForceOverrideAllowsProviderComplaint(t *testing.T) {
	t.Parallel()

	enableCalled := false
	ns := &notifyStoreAdapter{
		getNotifySubscriberFunc: func(_ context.Context, id, projectID string) (*domain.NotifySubscriber, error) {
			return &domain.NotifySubscriber{ID: id, ProjectID: projectID}, nil
		},
		getLatestNotifySuppressionEventFunc: func(_ context.Context, projectID, recipientType, recipientID, scope, channel string) (*domain.NotifySuppressionEvent, error) {
			return &domain.NotifySuppressionEvent{
				ID:            "evt_1",
				ProjectID:     projectID,
				RecipientType: recipientType,
				RecipientID:   recipientID,
				Scope:         scope,
				Channel:       channel,
				Action:        domain.NotifySuppressionActionSuppressed,
				Reason:        "provider_callback:email.bounced",
				Source:        domain.NotifySuppressionSourceProviderCallback,
			}, nil
		},
		enableNotificationChannelPreferenceFunc: func(_ context.Context, _, _, _, _ string) error {
			enableCalled = true
			return nil
		},
		createNotifySuppressionEventFunc: func(_ context.Context, event *domain.NotifySuppressionEvent) error {
			if event.Action != domain.NotifySuppressionActionUnsuppressed {
				t.Fatalf("event.Action = %q, want %q", event.Action, domain.NotifySuppressionActionUnsuppressed)
			}
			return nil
		},
	}

	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/subscribers/sub_1/suppressions/unsuppress", `{"channel":"email","scope":"global","force":true}`, "proj_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !enableCalled {
		t.Fatal("expected enable when force=true")
	}
}

func TestUpdateNotifyPreferencesScope_BlocksSelfServiceUnsuppressForProviderComplaint(t *testing.T) {
	t.Parallel()

	upsertCalled := false
	ns := &notifyStoreAdapter{
		getNotificationPreferenceFunc: func(_ context.Context, recipientType, recipientID, scope string) (*domain.NotificationPreference, error) {
			return &domain.NotificationPreference{
				RecipientType:    recipientType,
				RecipientID:      recipientID,
				Scope:            scope,
				ChannelPrefs:     []byte(`{"email":false}`),
				Timezone:         "UTC",
				DigestPolicy:     "immediate",
				CriticalOverride: true,
			}, nil
		},
		getLatestNotifySuppressionEventFunc: func(_ context.Context, projectID, recipientType, recipientID, scope, channel string) (*domain.NotifySuppressionEvent, error) {
			return &domain.NotifySuppressionEvent{
				ProjectID:     projectID,
				RecipientType: recipientType,
				RecipientID:   recipientID,
				Scope:         scope,
				Channel:       channel,
				Action:        domain.NotifySuppressionActionSuppressed,
				Reason:        "provider_callback:email.complained",
			}, nil
		},
		upsertNotificationPreferenceFunc: func(_ context.Context, _ *domain.NotificationPreference) error {
			upsertCalled = true
			return nil
		},
		listNotificationPreferencesFunc: func(_ context.Context, _, _ string) ([]domain.NotificationPreference, error) {
			return []domain.NotificationPreference{}, nil
		},
	}

	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)
	token, err := srv.createNotifySubscriberToken("sub_1", "proj_1", "", time.Hour)
	if err != nil {
		t.Fatalf("createNotifySubscriberToken() error = %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/v1/preferences/global", strings.NewReader(`{"channel_prefs":{"email":true}}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	if upsertCalled {
		t.Fatal("expected preference upsert to be blocked")
	}
}

func TestListNotifySuppressionEvents(t *testing.T) {
	t.Parallel()

	ns := &notifyStoreAdapter{
		getNotifySubscriberFunc: func(_ context.Context, id, projectID string) (*domain.NotifySubscriber, error) {
			return &domain.NotifySubscriber{ID: id, ProjectID: projectID}, nil
		},
		listNotifySuppressionEventsFunc: func(_ context.Context, projectID, recipientType, recipientID string, limit int, _ *time.Time) ([]domain.NotifySuppressionEvent, error) {
			if projectID != "proj_1" || recipientType != domain.NotifyRecipientTypeSubscriber || recipientID != "sub_1" || limit != 10 {
				t.Fatalf("unexpected list args project=%q type=%q id=%q limit=%d", projectID, recipientType, recipientID, limit)
			}
			return []domain.NotifySuppressionEvent{{
				ID:            "evt_1",
				ProjectID:     projectID,
				RecipientType: recipientType,
				RecipientID:   recipientID,
				Scope:         "global",
				Channel:       "email",
				Action:        domain.NotifySuppressionActionSuppressed,
				Source:        domain.NotifySuppressionSourceProviderCallback,
				CreatedAt:     time.Now().UTC(),
			}}, nil
		},
	}

	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/subscribers/sub_1/suppressions?limit=10", "", "proj_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "evt_1") {
		t.Fatalf("response body missing suppression event: %s", w.Body.String())
	}
}

func TestNotifySuppressionReasonRequiresManualOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reason string
		want   bool
	}{
		{name: "complaint callback", reason: "provider_callback:email.complained", want: true},
		{name: "bounce callback", reason: "provider_callback:email.bounced", want: true},
		{name: "provider callback delivered", reason: "provider_callback:email.delivered", want: false},
		{name: "manual reason", reason: "manual_unsuppress", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := notifySuppressionReasonRequiresManualOverride(tc.reason); got != tc.want {
				t.Fatalf("notifySuppressionReasonRequiresManualOverride(%q) = %v, want %v", tc.reason, got, tc.want)
			}
		})
	}
}

func TestNotifyChannelPrefExplicitEnableEmail(t *testing.T) {
	t.Parallel()

	if !notifyChannelPrefExplicitEnableEmail(json.RawMessage(`{"email":true}`)) {
		t.Fatal("notifyChannelPrefExplicitEnableEmail(email=true) = false, want true")
	}
	if notifyChannelPrefExplicitEnableEmail(json.RawMessage(`{"email":false}`)) {
		t.Fatal("notifyChannelPrefExplicitEnableEmail(email=false) = true, want false")
	}
	if notifyChannelPrefExplicitEnableEmail(json.RawMessage(`{"inbox":true}`)) {
		t.Fatal("notifyChannelPrefExplicitEnableEmail(missing email) = true, want false")
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

	hourly, ok := resolveNotifyDigestWindow(notifyDigestPolicyHourly, now, "")
	if !ok {
		t.Fatal("resolveNotifyDigestWindow(hourly) ok = false, want true")
	}
	if hourly.Format(time.RFC3339) != "2026-04-05T14:00:00Z" {
		t.Fatalf("hourly window = %s, want 2026-04-05T14:00:00Z", hourly.Format(time.RFC3339))
	}

	daily, ok := resolveNotifyDigestWindow(notifyDigestPolicyDaily, now, "")
	if !ok {
		t.Fatal("resolveNotifyDigestWindow(daily) ok = false, want true")
	}
	if daily.Format(time.RFC3339) != "2026-04-06T00:00:00Z" {
		t.Fatalf("daily window = %s, want 2026-04-06T00:00:00Z", daily.Format(time.RFC3339))
	}

	berlinDaily, ok := resolveNotifyDigestWindow(notifyDigestPolicyDaily, now, "Europe/Berlin")
	if !ok {
		t.Fatal("resolveNotifyDigestWindow(daily, tz) ok = false, want true")
	}
	if berlinDaily.Format(time.RFC3339) != "2026-04-05T22:00:00Z" {
		t.Fatalf("berlin daily window = %s, want 2026-04-05T22:00:00Z", berlinDaily.Format(time.RFC3339))
	}

	if _, ok := resolveNotifyDigestWindow("weekly", now, ""); ok {
		t.Fatal("resolveNotifyDigestWindow(weekly) ok = true, want false")
	}
}

func TestApplyNotifyDigestPolicyOverride(t *testing.T) {
	t.Parallel()

	if got := applyNotifyDigestPolicyOverride(notifyDigestPolicyHourly, nil); got != notifyDigestPolicyHourly {
		t.Fatalf("apply override (nil) = %q, want %q", got, notifyDigestPolicyHourly)
	}

	override := &domain.NotifyPolicyOverride{DigestPolicy: "daily"}
	if got := applyNotifyDigestPolicyOverride(notifyDigestPolicyHourly, override); got != notifyDigestPolicyDaily {
		t.Fatalf("apply override (daily) = %q, want %q", got, notifyDigestPolicyDaily)
	}

	override = &domain.NotifyPolicyOverride{DigestPolicy: "instant"}
	if got := applyNotifyDigestPolicyOverride(notifyDigestPolicyDaily, override); got != notifyDigestPolicyInstant {
		t.Fatalf("apply override (instant) = %q, want %q", got, notifyDigestPolicyInstant)
	}

	override = &domain.NotifyPolicyOverride{DigestPolicy: "weekly"}
	if got := applyNotifyDigestPolicyOverride(notifyDigestPolicyDaily, override); got != notifyDigestPolicyDaily {
		t.Fatalf("apply override (invalid) = %q, want %q", got, notifyDigestPolicyDaily)
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

func TestCreateNotifyPolicyOverride(t *testing.T) {
	t.Parallel()

	called := false
	ns := &notifyStoreAdapter{
		upsertNotifyPolicyOverrideFunc: func(_ context.Context, policy *domain.NotifyPolicyOverride) error {
			called = true
			if policy.ProjectID != "proj_1" {
				t.Fatalf("ProjectID = %q, want proj_1", policy.ProjectID)
			}
			if policy.ScopeType != domain.NotifyPolicyScopeCategory {
				t.Fatalf("ScopeType = %q, want %q", policy.ScopeType, domain.NotifyPolicyScopeCategory)
			}
			policy.ID = "pol_1"
			return nil
		},
	}
	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPost, "/v1/notify/policies", `{"scope_type":"category","scope_key":"billing","digest_policy":"daily"}`, "proj_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	if !called {
		t.Fatal("expected UpsertNotifyPolicyOverride to be called")
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["id"] != "pol_1" {
		t.Fatalf("id = %v, want pol_1", body["id"])
	}
}

func TestUpdateNotifyPolicyOverride(t *testing.T) {
	t.Parallel()

	updated := false
	ns := &notifyStoreAdapter{
		getNotifyPolicyOverrideFunc: func(_ context.Context, id, projectID string) (*domain.NotifyPolicyOverride, error) {
			if id != "pol_1" || projectID != "proj_1" {
				t.Fatalf("GetNotifyPolicyOverride args = (%q,%q), want (pol_1,proj_1)", id, projectID)
			}
			return &domain.NotifyPolicyOverride{ID: "pol_1", ProjectID: projectID, ScopeType: domain.NotifyPolicyScopeProject, ScopeKey: "*", Enabled: true}, nil
		},
		upsertNotifyPolicyOverrideFunc: func(_ context.Context, policy *domain.NotifyPolicyOverride) error {
			updated = true
			if policy.DigestPolicy != "hourly" {
				t.Fatalf("DigestPolicy = %q, want hourly", policy.DigestPolicy)
			}
			if !policy.Enabled {
				t.Fatal("Enabled = false, want true")
			}
			return nil
		},
	}
	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodPut, "/v1/notify/policies/pol_1", `{"digest_policy":"hourly","enabled":true}`, "proj_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !updated {
		t.Fatal("expected update upsert call")
	}
}

func TestListNotifyPolicyOverrides(t *testing.T) {
	t.Parallel()

	ns := &notifyStoreAdapter{
		listNotifyPolicyOverridesFunc: func(_ context.Context, projectID string, scopeType *string) ([]domain.NotifyPolicyOverride, error) {
			if projectID != "proj_1" {
				t.Fatalf("projectID = %q, want proj_1", projectID)
			}
			if scopeType == nil || *scopeType != "category" {
				t.Fatalf("scopeType = %v, want category", scopeType)
			}
			return []domain.NotifyPolicyOverride{{ID: "pol_1", ProjectID: projectID, ScopeType: domain.NotifyPolicyScopeCategory, ScopeKey: "billing", Enabled: true}}, nil
		},
	}
	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodGet, "/v1/notify/policies?scope_type=category", "", "proj_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var body []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("len(body) = %d, want 1", len(body))
	}
}

func TestDeleteNotifyPolicyOverride_NotFound(t *testing.T) {
	t.Parallel()

	ns := &notifyStoreAdapter{
		deleteNotifyPolicyOverrideFunc: func(_ context.Context, _, _ string) error {
			return store.ErrNotifyPolicyNotFound
		},
	}
	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedProjectRequest(http.MethodDelete, "/v1/notify/policies/pol_missing", "", "proj_1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotFound, w.Body.String())
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
