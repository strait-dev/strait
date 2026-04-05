package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestExtractNotifyResendMessageID_FromTags(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"type": "email.bounced",
		"data": map[string]any{
			"tags": []any{
				map[string]any{"name": "other", "value": "x"},
				map[string]any{"name": "strait_message_id", "value": "msg_123"},
			},
		},
	}

	if got := extractNotifyResendMessageID(payload); got != "msg_123" {
		t.Fatalf("extractNotifyResendMessageID() = %q, want msg_123", got)
	}
}

func TestResolveNotifyResendCallbackOutcome(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	status, fields, suppress := resolveNotifyResendCallbackOutcome("email.bounced", now)
	if status != domain.NotifyMessageStatusBounced {
		t.Fatalf("bounce status = %q, want %q", status, domain.NotifyMessageStatusBounced)
	}
	if fields["bounced_at"] == nil {
		t.Fatal("bounce fields missing bounced_at")
	}
	if !suppress {
		t.Fatal("bounce should suppress email")
	}

	status, fields, suppress = resolveNotifyResendCallbackOutcome("email.delivered", now)
	if status != domain.NotifyMessageStatusDelivered {
		t.Fatalf("delivered status = %q, want %q", status, domain.NotifyMessageStatusDelivered)
	}
	if fields["delivered_at"] == nil {
		t.Fatal("delivered fields missing delivered_at")
	}
	if suppress {
		t.Fatal("delivered should not suppress email")
	}
}

func TestSuppressNotifyRecipientEmail_UpsertsPreference(t *testing.T) {
	t.Parallel()

	var persisted *domain.NotificationPreference
	ns := &notifyStoreAdapter{
		getNotificationPreferenceFunc: func(_ context.Context, _, _, _ string) (*domain.NotificationPreference, error) {
			return &domain.NotificationPreference{
				RecipientType: domain.NotifyRecipientTypeSubscriber,
				RecipientID:   "sub_1",
				Scope:         "global",
				ChannelPrefs:  []byte(`{"inbox":true,"email":true}`),
			}, nil
		},
		upsertNotificationPreferenceFunc: func(_ context.Context, pref *domain.NotificationPreference) error {
			cloned := *pref
			persisted = &cloned
			return nil
		},
	}

	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)
	err := srv.suppressNotifyRecipientEmail(context.Background(), ns, &domain.NotificationMessage{
		RecipientType: domain.NotifyRecipientTypeSubscriber,
		RecipientID:   "sub_1",
	})
	if err != nil {
		t.Fatalf("suppressNotifyRecipientEmail() error = %v", err)
	}
	if persisted == nil {
		t.Fatal("expected preference upsert")
	}
	if string(persisted.ChannelPrefs) != `{"email":false,"inbox":true}` && string(persisted.ChannelPrefs) != `{"inbox":true,"email":false}` {
		t.Fatalf("channel prefs = %s, want email disabled", string(persisted.ChannelPrefs))
	}
}

func TestNotifyResendProviderCallback_RejectsMissingWebhookSecret(t *testing.T) {
	t.Parallel()

	ns := &notifyStoreAdapter{
		getNotificationProviderFunc: func(_ context.Context, _, _ string) (*domain.NotificationProvider, error) {
			return &domain.NotificationProvider{ID: "provider_1", ProjectID: "proj_1", Provider: "resend", ConfigEnc: []byte(`{"api_key":"rk_test"}`)}, nil
		},
	}
	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/notify/providers/proj_1/provider_1/callbacks/resend", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
