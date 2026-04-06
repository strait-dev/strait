package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type notifySESFeedbackStoreMock struct {
	getMessageByIDFunc         func(ctx context.Context, id string) (*domain.NotificationMessage, error)
	updateMessageStatusFunc    func(ctx context.Context, id, projectID, fromStatus, toStatus string, fields map[string]any) error
	disableChannelPrefFunc     func(ctx context.Context, recipientType, recipientID, scope, channel string) error
	createSuppressionEventFunc func(ctx context.Context, event *domain.NotifySuppressionEvent) error
	recordCallbackReceiptFunc  func(ctx context.Context, projectID, providerID, provider, callbackID, eventType, messageID, payloadHash string, expiresAt time.Time) (bool, error)
	deleteCallbackReceiptFunc  func(ctx context.Context, projectID, providerID, callbackID string) error
}

func (m *notifySESFeedbackStoreMock) GetNotificationMessageByID(ctx context.Context, id string) (*domain.NotificationMessage, error) {
	if m.getMessageByIDFunc == nil {
		return nil, store.ErrNotificationMessageNotFound
	}
	return m.getMessageByIDFunc(ctx, id)
}

func (m *notifySESFeedbackStoreMock) UpdateNotificationMessageStatus(ctx context.Context, id, projectID, fromStatus, toStatus string, fields map[string]any) error {
	if m.updateMessageStatusFunc == nil {
		return nil
	}
	return m.updateMessageStatusFunc(ctx, id, projectID, fromStatus, toStatus, fields)
}

func (m *notifySESFeedbackStoreMock) DisableNotificationChannelPreference(ctx context.Context, recipientType, recipientID, scope, channel string) error {
	if m.disableChannelPrefFunc == nil {
		return nil
	}
	return m.disableChannelPrefFunc(ctx, recipientType, recipientID, scope, channel)
}

func (m *notifySESFeedbackStoreMock) CreateNotifySuppressionEvent(ctx context.Context, event *domain.NotifySuppressionEvent) error {
	if m.createSuppressionEventFunc == nil {
		return nil
	}
	return m.createSuppressionEventFunc(ctx, event)
}

func (m *notifySESFeedbackStoreMock) RecordNotifyProviderCallbackReceipt(
	ctx context.Context,
	projectID, providerID, provider, callbackID, eventType, messageID, payloadHash string,
	expiresAt time.Time,
) (bool, error) {
	if m.recordCallbackReceiptFunc == nil {
		return true, nil
	}
	return m.recordCallbackReceiptFunc(ctx, projectID, providerID, provider, callbackID, eventType, messageID, payloadHash, expiresAt)
}

func (m *notifySESFeedbackStoreMock) DeleteNotifyProviderCallbackReceipt(ctx context.Context, projectID, providerID, callbackID string) error {
	if m.deleteCallbackReceiptFunc == nil {
		return nil
	}
	return m.deleteCallbackReceiptFunc(ctx, projectID, providerID, callbackID)
}

type notifySESFeedbackSQSClientMock struct {
	receiveOutput *sqs.ReceiveMessageOutput
	receiveErr    error
	deleted       []string
}

func (m *notifySESFeedbackSQSClientMock) ReceiveMessage(_ context.Context, _ *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	return m.receiveOutput, m.receiveErr
}

func (m *notifySESFeedbackSQSClientMock) DeleteMessage(_ context.Context, params *sqs.DeleteMessageInput, _ ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	m.deleted = append(m.deleted, awsv2.ToString(params.ReceiptHandle))
	return &sqs.DeleteMessageOutput{}, nil
}

func TestNotifySESFeedbackConsumerPollOnce_BounceSuppressesSubscriber(t *testing.T) {
	t.Parallel()

	msg := &domain.NotificationMessage{
		ID:            "msg_1",
		ProjectID:     "proj_1",
		ProviderID:    "provider_1",
		RecipientType: domain.NotifyRecipientTypeSubscriber,
		RecipientID:   "sub_1",
		Status:        domain.NotifyMessageStatusProcessing,
	}

	updated := false
	disabled := false
	eventLogged := false
	storeMock := &notifySESFeedbackStoreMock{
		getMessageByIDFunc: func(_ context.Context, id string) (*domain.NotificationMessage, error) {
			if id != "msg_1" {
				t.Fatalf("GetNotificationMessageByID id = %q, want msg_1", id)
			}
			return msg, nil
		},
		recordCallbackReceiptFunc: func(_ context.Context, projectID, providerID, provider, callbackID, eventType, messageID, payloadHash string, _ time.Time) (bool, error) {
			if projectID != "proj_1" || providerID != "provider_1" || provider != "ses" {
				t.Fatalf("unexpected receipt identity project=%q providerID=%q provider=%q", projectID, providerID, provider)
			}
			if callbackID != "sns_cb_1" {
				t.Fatalf("callbackID = %q, want sns_cb_1", callbackID)
			}
			if eventType != "bounce" || messageID != "msg_1" || payloadHash == "" {
				t.Fatalf("unexpected receipt payload eventType=%q messageID=%q hash=%q", eventType, messageID, payloadHash)
			}
			return true, nil
		},
		updateMessageStatusFunc: func(_ context.Context, id, projectID, fromStatus, toStatus string, fields map[string]any) error {
			updated = true
			if id != "msg_1" || projectID != "proj_1" {
				t.Fatalf("unexpected status update target id=%q project=%q", id, projectID)
			}
			if fromStatus != domain.NotifyMessageStatusProcessing || toStatus != domain.NotifyMessageStatusBounced {
				t.Fatalf("unexpected status transition %q -> %q", fromStatus, toStatus)
			}
			if fields["suppression_reason"] != "provider_callback:ses.bounce" {
				t.Fatalf("suppression_reason = %v, want provider_callback:ses.bounce", fields["suppression_reason"])
			}
			if fields["bounced_at"] == nil {
				t.Fatal("expected bounced_at field")
			}
			return nil
		},
		disableChannelPrefFunc: func(_ context.Context, recipientType, recipientID, scope, channel string) error {
			disabled = true
			if recipientType != domain.NotifyRecipientTypeSubscriber || recipientID != "sub_1" || scope != "global" || channel != "email" {
				t.Fatalf("unexpected disable args type=%q id=%q scope=%q channel=%q", recipientType, recipientID, scope, channel)
			}
			return nil
		},
		createSuppressionEventFunc: func(_ context.Context, event *domain.NotifySuppressionEvent) error {
			eventLogged = true
			if event.Action != domain.NotifySuppressionActionSuppressed || event.Reason != "provider_callback:ses.bounce" {
				t.Fatalf("unexpected suppression event action=%q reason=%q", event.Action, event.Reason)
			}
			return nil
		},
	}

	sqsMock := &notifySESFeedbackSQSClientMock{
		receiveOutput: &sqs.ReceiveMessageOutput{Messages: []sqstypes.Message{{
			MessageId:     awsv2.String("sqs_1"),
			ReceiptHandle: awsv2.String("rh_1"),
			Body:          awsv2.String(mustNotifySESFeedbackSNSBody(t, "sns_cb_1", "Bounce", "msg_1", "proj_1")),
		}}},
	}

	consumer := NewNotifySESFeedbackConsumerWithClient(storeMock, sqsMock, NotifySESFeedbackConsumerConfig{QueueURL: "queue-url"})
	processed, err := consumer.pollOnce(context.Background())
	if err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if !updated || !disabled || !eventLogged {
		t.Fatalf("expected updated=%v disabled=%v eventLogged=%v to all be true", updated, disabled, eventLogged)
	}
	if len(sqsMock.deleted) != 1 || sqsMock.deleted[0] != "rh_1" {
		t.Fatalf("deleted receipts = %#v, want [rh_1]", sqsMock.deleted)
	}
}

func TestNotifySESFeedbackConsumerPollOnce_DuplicateReceiptIsAcked(t *testing.T) {
	t.Parallel()

	updateCalled := false
	storeMock := &notifySESFeedbackStoreMock{
		getMessageByIDFunc: func(_ context.Context, _ string) (*domain.NotificationMessage, error) {
			return &domain.NotificationMessage{ID: "msg_1", ProjectID: "proj_1", Status: domain.NotifyMessageStatusProcessing}, nil
		},
		recordCallbackReceiptFunc: func(_ context.Context, _, _, _, _, _, _, _ string, _ time.Time) (bool, error) {
			return false, nil
		},
		updateMessageStatusFunc: func(_ context.Context, _, _, _, _ string, _ map[string]any) error {
			updateCalled = true
			return nil
		},
	}
	sqsMock := &notifySESFeedbackSQSClientMock{
		receiveOutput: &sqs.ReceiveMessageOutput{Messages: []sqstypes.Message{{
			MessageId:     awsv2.String("sqs_1"),
			ReceiptHandle: awsv2.String("rh_1"),
			Body:          awsv2.String(mustNotifySESFeedbackSNSBody(t, "sns_cb_1", "Delivery", "msg_1", "proj_1")),
		}}},
	}

	consumer := NewNotifySESFeedbackConsumerWithClient(storeMock, sqsMock, NotifySESFeedbackConsumerConfig{QueueURL: "queue-url"})
	processed, err := consumer.pollOnce(context.Background())
	if err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if updateCalled {
		t.Fatal("expected duplicate receipt to skip status update")
	}
}

func TestNotifySESFeedbackConsumerPollOnce_MalformedPayloadIsAcked(t *testing.T) {
	t.Parallel()

	sqsMock := &notifySESFeedbackSQSClientMock{
		receiveOutput: &sqs.ReceiveMessageOutput{Messages: []sqstypes.Message{{
			MessageId:     awsv2.String("sqs_1"),
			ReceiptHandle: awsv2.String("rh_1"),
			Body:          awsv2.String("not-json"),
		}}},
	}

	consumer := NewNotifySESFeedbackConsumerWithClient(&notifySESFeedbackStoreMock{}, sqsMock, NotifySESFeedbackConsumerConfig{QueueURL: "queue-url"})
	processed, err := consumer.pollOnce(context.Background())
	if err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
}

func TestNotifySESFeedbackConsumerPollOnce_StatusConflictUsesLatest(t *testing.T) {
	t.Parallel()

	calls := 0
	storeMock := &notifySESFeedbackStoreMock{
		getMessageByIDFunc: func(_ context.Context, _ string) (*domain.NotificationMessage, error) {
			calls++
			if calls == 1 {
				return &domain.NotificationMessage{ID: "msg_1", ProjectID: "proj_1", Status: domain.NotifyMessageStatusProcessing}, nil
			}
			return &domain.NotificationMessage{ID: "msg_1", ProjectID: "proj_1", Status: domain.NotifyMessageStatusDelivered}, nil
		},
		recordCallbackReceiptFunc: func(_ context.Context, _, _, _, _, _, _, _ string, _ time.Time) (bool, error) {
			return true, nil
		},
		updateMessageStatusFunc: func(_ context.Context, _, _, _, _ string, _ map[string]any) error {
			return store.ErrNotificationMessageStatusConflict
		},
	}
	sqsMock := &notifySESFeedbackSQSClientMock{
		receiveOutput: &sqs.ReceiveMessageOutput{Messages: []sqstypes.Message{{
			MessageId:     awsv2.String("sqs_1"),
			ReceiptHandle: awsv2.String("rh_1"),
			Body:          awsv2.String(mustNotifySESFeedbackSNSBody(t, "sns_cb_1", "Delivery", "msg_1", "proj_1")),
		}}},
	}

	consumer := NewNotifySESFeedbackConsumerWithClient(storeMock, sqsMock, NotifySESFeedbackConsumerConfig{QueueURL: "queue-url"})
	processed, err := consumer.pollOnce(context.Background())
	if err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
}

func TestParseNotifySESEvent_RawSESBody(t *testing.T) {
	t.Parallel()

	body := mustNotifySESFeedbackEventBody(t, "Delivery", "msg_1", "proj_1")
	event, callbackID, eventType, err := parseNotifySESEvent([]byte(body))
	if err != nil {
		t.Fatalf("parseNotifySESEvent() error = %v", err)
	}
	if callbackID != "" {
		t.Fatalf("callbackID = %q, want empty", callbackID)
	}
	if eventType != "delivery" {
		t.Fatalf("eventType = %q, want delivery", eventType)
	}
	if event.straitMessageID() != "msg_1" {
		t.Fatalf("straitMessageID = %q, want msg_1", event.straitMessageID())
	}
}

func mustNotifySESFeedbackSNSBody(t *testing.T, callbackID, eventType, straitMessageID, projectID string) string {
	t.Helper()
	event := mustNotifySESFeedbackEventBody(t, eventType, straitMessageID, projectID)
	envelope, err := json.Marshal(map[string]any{
		"Type":      "Notification",
		"MessageId": callbackID,
		"Message":   event,
	})
	if err != nil {
		t.Fatalf("marshal sns envelope: %v", err)
	}
	return string(envelope)
}

func mustNotifySESFeedbackEventBody(t *testing.T, eventType, straitMessageID, projectID string) string {
	t.Helper()
	event := map[string]any{
		"eventType": eventType,
		"mail": map[string]any{
			"messageId": "ses_msg_1",
			"timestamp": "2026-04-06T10:00:00Z",
			"tags": map[string]any{
				"strait_message_id": []string{straitMessageID},
				"strait_project_id": []string{projectID},
			},
		},
		"bounce":    map[string]any{"timestamp": "2026-04-06T10:01:00Z"},
		"complaint": map[string]any{"timestamp": "2026-04-06T10:01:00Z"},
		"delivery":  map[string]any{"timestamp": "2026-04-06T10:01:00Z"},
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal ses event: %v", err)
	}
	return string(encoded)
}

func TestResolveNotifySESFeedbackOutcome_Unsupported(t *testing.T) {
	t.Parallel()

	status, fields, suppress, reason := resolveNotifySESFeedbackOutcome("open", time.Now())
	if status != "" || fields != nil || suppress || reason != "" {
		t.Fatalf("unexpected unsupported outcome status=%q fields=%v suppress=%v reason=%q", status, fields, suppress, reason)
	}
}

func TestNotifySESFeedbackConsumerPollOnce_TransientStoreErrorNotAcked(t *testing.T) {
	t.Parallel()

	sqsMock := &notifySESFeedbackSQSClientMock{
		receiveOutput: &sqs.ReceiveMessageOutput{Messages: []sqstypes.Message{{
			MessageId:     awsv2.String("sqs_1"),
			ReceiptHandle: awsv2.String("rh_1"),
			Body:          awsv2.String(mustNotifySESFeedbackSNSBody(t, "sns_cb_1", "Delivery", "msg_1", "proj_1")),
		}}},
	}
	storeMock := &notifySESFeedbackStoreMock{
		getMessageByIDFunc: func(_ context.Context, _ string) (*domain.NotificationMessage, error) {
			return nil, errors.New("db down")
		},
	}

	consumer := NewNotifySESFeedbackConsumerWithClient(storeMock, sqsMock, NotifySESFeedbackConsumerConfig{QueueURL: "queue-url"})
	processed, err := consumer.pollOnce(context.Background())
	if err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed = %d, want 0 (message should remain for retry)", processed)
	}
}
