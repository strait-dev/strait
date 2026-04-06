package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
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

	status, fields, suppress = resolveNotifyResendCallbackOutcome("email.complained", now)
	if status != domain.NotifyMessageStatusFailed {
		t.Fatalf("complaint status = %q, want %q", status, domain.NotifyMessageStatusFailed)
	}
	if fields["suppression_reason"] == nil {
		t.Fatal("complaint fields missing suppression_reason")
	}
	if !suppress {
		t.Fatal("complaint should suppress email")
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

func TestShouldApplyNotifyProviderCallbackTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		next    string
		want    bool
	}{
		{name: "processing to delivered", current: domain.NotifyMessageStatusProcessing, next: domain.NotifyMessageStatusDelivered, want: true},
		{name: "same status duplicate", current: domain.NotifyMessageStatusDelivered, next: domain.NotifyMessageStatusDelivered, want: false},
		{name: "terminal delivered ignored", current: domain.NotifyMessageStatusDelivered, next: domain.NotifyMessageStatusBounced, want: false},
		{name: "terminal failed ignored", current: domain.NotifyMessageStatusFailed, next: domain.NotifyMessageStatusDelivered, want: false},
		{name: "missing next status", current: domain.NotifyMessageStatusProcessing, next: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldApplyNotifyProviderCallbackTransition(tc.current, tc.next); got != tc.want {
				t.Fatalf("shouldApplyNotifyProviderCallbackTransition(%q, %q) = %v, want %v", tc.current, tc.next, got, tc.want)
			}
		})
	}
}

func TestSuppressNotifyRecipientEmail_DisablesEmailChannel(t *testing.T) {
	t.Parallel()

	called := false
	var gotRecipientType, gotRecipientID, gotScope, gotChannel string

	ns := &notifyStoreAdapter{
		disableNotificationChannelPreferenceFunc: func(_ context.Context, recipientType, recipientID, scope, channel string) error {
			called = true
			gotRecipientType = recipientType
			gotRecipientID = recipientID
			gotScope = scope
			gotChannel = channel
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
	if !called {
		t.Fatal("expected DisableNotificationChannelPreference call")
	}
	if gotRecipientType != domain.NotifyRecipientTypeSubscriber || gotRecipientID != "sub_1" || gotScope != "global" || gotChannel != "email" {
		t.Fatalf("unexpected disable args: type=%q id=%q scope=%q channel=%q", gotRecipientType, gotRecipientID, gotScope, gotChannel)
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

func TestNotifyResendProviderCallback_IgnoresProviderMismatch(t *testing.T) {
	t.Parallel()

	const (
		webhookSecret = "whsec_c2VjcmV0"
		payload       = `{"type":"email.delivered","data":{"tags":[{"name":"strait_message_id","value":"msg_1"}]}}`
	)

	updateCalls := 0
	ns := &notifyStoreAdapter{
		getNotificationProviderFunc: func(_ context.Context, _, _ string) (*domain.NotificationProvider, error) {
			return &domain.NotificationProvider{ID: "provider_1", ProjectID: "proj_1", Provider: "resend", ConfigEnc: []byte(`{"webhook_secret":"` + webhookSecret + `"}`)}, nil
		},
		getNotificationMessageFunc: func(_ context.Context, _, _ string) (*domain.NotificationMessage, error) {
			return &domain.NotificationMessage{ID: "msg_1", ProjectID: "proj_1", ProviderID: "provider_other", Status: domain.NotifyMessageStatusProcessing}, nil
		},
		updateNotificationMessageStatusFunc: func(_ context.Context, _, _, _, _ string, _ map[string]any) error {
			updateCalls++
			return nil
		},
	}

	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := signedResendCallbackRequest(t, "/v1/notify/providers/proj_1/provider_1/callbacks/resend", webhookSecret, payload)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	if updateCalls != 0 {
		t.Fatalf("update calls = %d, want 0", updateCalls)
	}
}

func TestNotifyResendProviderCallback_DuplicateComplaintMaintainsSuppression(t *testing.T) {
	t.Parallel()

	const (
		webhookSecret = "whsec_c2VjcmV0"
		payload       = `{"type":"email.complained","data":{"tags":[{"name":"strait_message_id","value":"msg_1"}]}}`
	)

	updateCalls := 0
	suppressionCalls := 0
	ns := &notifyStoreAdapter{
		getNotificationProviderFunc: func(_ context.Context, _, _ string) (*domain.NotificationProvider, error) {
			return &domain.NotificationProvider{ID: "provider_1", ProjectID: "proj_1", Provider: "resend", ConfigEnc: []byte(`{"webhook_secret":"` + webhookSecret + `"}`)}, nil
		},
		getNotificationMessageFunc: func(_ context.Context, _, _ string) (*domain.NotificationMessage, error) {
			return &domain.NotificationMessage{
				ID:            "msg_1",
				ProjectID:     "proj_1",
				ProviderID:    "provider_1",
				Status:        domain.NotifyMessageStatusFailed,
				RecipientType: domain.NotifyRecipientTypeSubscriber,
				RecipientID:   "sub_1",
			}, nil
		},
		updateNotificationMessageStatusFunc: func(_ context.Context, _, _, _, _ string, _ map[string]any) error {
			updateCalls++
			return nil
		},
		disableNotificationChannelPreferenceFunc: func(_ context.Context, recipientType, recipientID, scope, channel string) error {
			suppressionCalls++
			if recipientType != domain.NotifyRecipientTypeSubscriber || recipientID != "sub_1" || scope != "global" || channel != "email" {
				t.Fatalf("unexpected suppression args type=%q id=%q scope=%q channel=%q", recipientType, recipientID, scope, channel)
			}
			return nil
		},
	}

	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := signedResendCallbackRequest(t, "/v1/notify/providers/proj_1/provider_1/callbacks/resend", webhookSecret, payload)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	if updateCalls != 0 {
		t.Fatalf("update calls = %d, want 0", updateCalls)
	}
	if suppressionCalls != 1 {
		t.Fatalf("suppression calls = %d, want 1", suppressionCalls)
	}
}

func TestNotifyResendProviderCallback_DuplicateBounceMaintainsSuppression(t *testing.T) {
	t.Parallel()

	const (
		webhookSecret = "whsec_c2VjcmV0"
		payload       = `{"type":"email.bounced","data":{"tags":[{"name":"strait_message_id","value":"msg_1"}]}}`
	)

	updateCalls := 0
	suppressionCalls := 0
	ns := &notifyStoreAdapter{
		getNotificationProviderFunc: func(_ context.Context, _, _ string) (*domain.NotificationProvider, error) {
			return &domain.NotificationProvider{ID: "provider_1", ProjectID: "proj_1", Provider: "resend", ConfigEnc: []byte(`{"webhook_secret":"` + webhookSecret + `"}`)}, nil
		},
		getNotificationMessageFunc: func(_ context.Context, _, _ string) (*domain.NotificationMessage, error) {
			return &domain.NotificationMessage{
				ID:            "msg_1",
				ProjectID:     "proj_1",
				ProviderID:    "provider_1",
				Status:        domain.NotifyMessageStatusBounced,
				RecipientType: domain.NotifyRecipientTypeSubscriber,
				RecipientID:   "sub_1",
			}, nil
		},
		updateNotificationMessageStatusFunc: func(_ context.Context, _, _, _, _ string, _ map[string]any) error {
			updateCalls++
			return nil
		},
		disableNotificationChannelPreferenceFunc: func(_ context.Context, _, _, _, _ string) error {
			suppressionCalls++
			return nil
		},
	}

	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := signedResendCallbackRequest(t, "/v1/notify/providers/proj_1/provider_1/callbacks/resend", webhookSecret, payload)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	if updateCalls != 0 {
		t.Fatalf("update calls = %d, want 0", updateCalls)
	}
	if suppressionCalls != 1 {
		t.Fatalf("suppression calls = %d, want 1", suppressionCalls)
	}
}

func TestNotifyResendProviderCallback_UpdatesStatusAndSuppressesOnBounce(t *testing.T) {
	t.Parallel()

	const (
		webhookSecret = "whsec_c2VjcmV0"
		payload       = `{"type":"email.bounced","data":{"tags":[{"name":"strait_message_id","value":"msg_1"}]}}`
	)

	updatedFrom := ""
	updatedTo := ""
	suppressionCalls := 0
	ns := &notifyStoreAdapter{
		getNotificationProviderFunc: func(_ context.Context, _, _ string) (*domain.NotificationProvider, error) {
			return &domain.NotificationProvider{ID: "provider_1", ProjectID: "proj_1", Provider: "resend", ConfigEnc: []byte(`{"webhook_secret":"` + webhookSecret + `"}`)}, nil
		},
		getNotificationMessageFunc: func(_ context.Context, _, _ string) (*domain.NotificationMessage, error) {
			return &domain.NotificationMessage{
				ID:            "msg_1",
				ProjectID:     "proj_1",
				ProviderID:    "provider_1",
				Status:        domain.NotifyMessageStatusProcessing,
				RecipientType: domain.NotifyRecipientTypeSubscriber,
				RecipientID:   "sub_1",
			}, nil
		},
		updateNotificationMessageStatusFunc: func(_ context.Context, _, _, fromStatus, toStatus string, fields map[string]any) error {
			updatedFrom = fromStatus
			updatedTo = toStatus
			if fields["bounced_at"] == nil {
				t.Fatal("expected bounced_at field")
			}
			return nil
		},
		disableNotificationChannelPreferenceFunc: func(_ context.Context, _, _, _, _ string) error {
			suppressionCalls++
			return nil
		},
	}

	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := signedResendCallbackRequest(t, "/v1/notify/providers/proj_1/provider_1/callbacks/resend", webhookSecret, payload)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	if updatedFrom != domain.NotifyMessageStatusProcessing || updatedTo != domain.NotifyMessageStatusBounced {
		t.Fatalf("updated transition = %q -> %q, want %q -> %q", updatedFrom, updatedTo, domain.NotifyMessageStatusProcessing, domain.NotifyMessageStatusBounced)
	}
	if suppressionCalls != 1 {
		t.Fatalf("suppression calls = %d, want 1", suppressionCalls)
	}
}

func TestNotifyResendProviderCallback_ConflictBecomesIgnored(t *testing.T) {
	t.Parallel()

	const (
		webhookSecret = "whsec_c2VjcmV0"
		payload       = `{"type":"email.delivered","data":{"tags":[{"name":"strait_message_id","value":"msg_1"}]}}`
	)

	getCalls := 0
	ns := &notifyStoreAdapter{
		getNotificationProviderFunc: func(_ context.Context, _, _ string) (*domain.NotificationProvider, error) {
			return &domain.NotificationProvider{ID: "provider_1", ProjectID: "proj_1", Provider: "resend", ConfigEnc: []byte(`{"webhook_secret":"` + webhookSecret + `"}`)}, nil
		},
		getNotificationMessageFunc: func(_ context.Context, _, _ string) (*domain.NotificationMessage, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.NotificationMessage{ID: "msg_1", ProjectID: "proj_1", ProviderID: "provider_1", Status: domain.NotifyMessageStatusProcessing}, nil
			}
			return &domain.NotificationMessage{ID: "msg_1", ProjectID: "proj_1", ProviderID: "provider_1", Status: domain.NotifyMessageStatusDelivered}, nil
		},
		updateNotificationMessageStatusFunc: func(_ context.Context, _, _, _, _ string, _ map[string]any) error {
			return store.ErrNotificationMessageStatusConflict
		},
	}

	srv := newTestServer(t, &notifyAPIStore{APIStoreMock: &APIStoreMock{}, NotifyStore: ns}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := signedResendCallbackRequest(t, "/v1/notify/providers/proj_1/provider_1/callbacks/resend", webhookSecret, payload)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	if getCalls != 2 {
		t.Fatalf("get notification message calls = %d, want 2", getCalls)
	}
}

func signedResendCallbackRequest(t *testing.T, path, webhookSecret, payload string) *http.Request {
	t.Helper()

	timestamp := time.Now().UTC().Unix()
	svixID := "msg_abc123"
	signature := signResendPayload(t, webhookSecret, svixID, fmt.Sprintf("%d", timestamp), payload)

	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("svix-id", svixID)
	r.Header.Set("svix-timestamp", fmt.Sprintf("%d", timestamp))
	r.Header.Set("svix-signature", signature)
	return r
}

func signResendPayload(t *testing.T, webhookSecret, svixID, timestamp, payload string) string {
	t.Helper()

	secret := strings.TrimPrefix(webhookSecret, "whsec_")
	decodedSecret, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		t.Fatalf("decode webhook secret: %v", err)
	}

	signedContent := fmt.Sprintf("%s.%s.%s", svixID, timestamp, payload)
	h := hmac.New(sha256.New, decodedSecret)
	_, _ = h.Write([]byte(signedContent))
	sig := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return "v1," + sig
}
