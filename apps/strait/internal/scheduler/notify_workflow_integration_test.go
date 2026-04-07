//go:build integration

package scheduler_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/scheduler"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type integrationNotifySQSClient struct {
	mu      sync.Mutex
	outputs []*sqs.ReceiveMessageOutput
	deleted []string
}

func (m *integrationNotifySQSClient) ReceiveMessage(_ context.Context, _ *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.outputs) == 0 {
		return &sqs.ReceiveMessageOutput{}, nil
	}
	out := m.outputs[0]
	m.outputs = m.outputs[1:]
	return out, nil
}

func (m *integrationNotifySQSClient) DeleteMessage(_ context.Context, params *sqs.DeleteMessageInput, _ ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, awsv2.ToString(params.ReceiptHandle))
	return &sqs.DeleteMessageOutput{}, nil
}

func (m *integrationNotifySQSClient) deletedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.deleted)
}

func TestIntegration_NotifyWorkflow_InboxDispatchAndSESBounceFeedback(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)
	testDB := getTestDB(t)

	projectID := "proj-notify-e2e"
	subscriber := &domain.NotifySubscriber{
		ProjectID:  projectID,
		ExternalID: "user-e2e",
		Email:      "notify-e2e@example.com",
		Status:     domain.NotifySubscriberStatusActive,
	}
	if err := st.UpsertNotifySubscriber(ctx, subscriber); err != nil {
		t.Fatalf("UpsertNotifySubscriber() error = %v", err)
	}

	now := time.Now().UTC()
	inboxMsg := &domain.NotificationMessage{
		ProjectID:       projectID,
		RecipientType:   domain.NotifyRecipientTypeSubscriber,
		RecipientID:     subscriber.ID,
		Channel:         "inbox",
		Status:          domain.NotifyMessageStatusScheduled,
		Attempts:        0,
		RenderedContent: []byte(`{"title":"Build complete","body":"Your workflow has finished."}`),
		ScheduledAt:     &now,
	}
	if err := st.CreateNotificationMessage(ctx, inboxMsg); err != nil {
		t.Fatalf("CreateNotificationMessage(inbox) error = %v", err)
	}

	emailMsg := &domain.NotificationMessage{
		ProjectID:       projectID,
		RecipientType:   domain.NotifyRecipientTypeSubscriber,
		RecipientID:     subscriber.ID,
		Channel:         "email",
		Status:          domain.NotifyMessageStatusProcessing,
		Attempts:        1,
		RenderedContent: []byte(`{"subject":"Approval pending","text_body":"Action required"}`),
	}
	if err := st.CreateNotificationMessage(ctx, emailMsg); err != nil {
		t.Fatalf("CreateNotificationMessage(email) error = %v", err)
	}

	dispatcher := scheduler.NewNotifyDispatcher(st, 10*time.Millisecond, scheduler.NotifyEmailDefaults{Provider: "ses", FromEmail: "noreply@strait.dev"})
	dispatchCtx, dispatchCancel := context.WithCancel(ctx)
	dispatchDone := make(chan struct{})
	go func() {
		dispatcher.Run(dispatchCtx)
		close(dispatchDone)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		msg, err := st.GetNotificationMessage(ctx, inboxMsg.ID, projectID)
		if err != nil {
			t.Fatalf("GetNotificationMessage(inbox) error = %v", err)
		}
		if msg.Status == domain.NotifyMessageStatusDelivered {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for inbox message delivery, last status=%s", msg.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}

	dispatchCancel()
	<-dispatchDone

	items, err := st.ListInboxItems(ctx, domain.NotifyRecipientTypeSubscriber, subscriber.ID, nil, 10, nil)
	if err != nil {
		t.Fatalf("ListInboxItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListInboxItems() len = %d, want 1", len(items))
	}
	if items[0].MessageID != inboxMsg.ID {
		t.Fatalf("InboxItem.MessageID = %q, want %q", items[0].MessageID, inboxMsg.ID)
	}

	feedbackBody := mustSESFeedbackSNSBody(t, "sns_cb_dup", "Bounce", emailMsg.ID, projectID)
	sqsClient := &integrationNotifySQSClient{
		outputs: []*sqs.ReceiveMessageOutput{{
			Messages: []sqstypes.Message{
				{MessageId: awsv2.String("sqs_1"), ReceiptHandle: awsv2.String("rh_1"), Body: awsv2.String(feedbackBody)},
				{MessageId: awsv2.String("sqs_2"), ReceiptHandle: awsv2.String("rh_2"), Body: awsv2.String(feedbackBody)},
			},
		}},
	}
	consumer := scheduler.NewNotifySESFeedbackConsumerWithClient(st, sqsClient, scheduler.NotifySESFeedbackConsumerConfig{QueueURL: "queue-url", PollInterval: 10 * time.Millisecond})

	feedbackCtx, feedbackCancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer feedbackCancel()
	consumer.Run(feedbackCtx)

	updatedEmail, err := st.GetNotificationMessage(ctx, emailMsg.ID, projectID)
	if err != nil {
		t.Fatalf("GetNotificationMessage(email) error = %v", err)
	}
	if updatedEmail.Status != domain.NotifyMessageStatusBounced {
		t.Fatalf("email status = %s, want %s", updatedEmail.Status, domain.NotifyMessageStatusBounced)
	}
	if updatedEmail.SuppressionReason != "provider_callback:ses.bounce" {
		t.Fatalf("suppression_reason = %q, want provider_callback:ses.bounce", updatedEmail.SuppressionReason)
	}
	if updatedEmail.BouncedAt == nil {
		t.Fatal("BouncedAt is nil, want non-nil")
	}

	pref, err := st.GetNotificationPreference(ctx, domain.NotifyRecipientTypeSubscriber, subscriber.ID, "global")
	if err != nil {
		t.Fatalf("GetNotificationPreference() error = %v", err)
	}
	var channelPrefs map[string]bool
	if err := json.Unmarshal(pref.ChannelPrefs, &channelPrefs); err != nil {
		t.Fatalf("unmarshal channel prefs: %v", err)
	}
	if enabled, ok := channelPrefs["email"]; !ok || enabled {
		t.Fatalf("expected email channel disabled, got %+v", channelPrefs)
	}

	suppressionEvents, err := st.ListNotifySuppressionEvents(ctx, projectID, domain.NotifyRecipientTypeSubscriber, subscriber.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListNotifySuppressionEvents() error = %v", err)
	}
	if len(suppressionEvents) != 1 {
		t.Fatalf("suppression events len = %d, want 1", len(suppressionEvents))
	}

	var callbackReceipts int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM notify_provider_callback_receipts
		WHERE project_id = $1 AND callback_id = $2
	`, projectID, "sns_cb_dup").Scan(&callbackReceipts); err != nil {
		t.Fatalf("count callback receipts: %v", err)
	}
	if callbackReceipts != 1 {
		t.Fatalf("callback receipt count = %d, want 1", callbackReceipts)
	}
	if sqsClient.deletedCount() != 2 {
		t.Fatalf("deleted sqs messages = %d, want 2", sqsClient.deletedCount())
	}
}

func TestIntegration_NotifySESFeedbackConsumer_DeliveryAndComplaint(t *testing.T) {
	ctx := context.Background()
	st := intTestStore(t)
	intTestClean(t, ctx)

	projectID := "proj-notify-feedback-delivery-complaint"
	subscriber := &domain.NotifySubscriber{
		ProjectID:  projectID,
		ExternalID: "user-feedback",
		Email:      "notify-feedback@example.com",
		Status:     domain.NotifySubscriberStatusActive,
	}
	if err := st.UpsertNotifySubscriber(ctx, subscriber); err != nil {
		t.Fatalf("UpsertNotifySubscriber() error = %v", err)
	}

	deliveryMsg := &domain.NotificationMessage{
		ProjectID:       projectID,
		RecipientType:   domain.NotifyRecipientTypeSubscriber,
		RecipientID:     subscriber.ID,
		Channel:         "email",
		Status:          domain.NotifyMessageStatusProcessing,
		Attempts:        1,
		RenderedContent: []byte(`{"subject":"Delivery check","text_body":"ok"}`),
	}
	if err := st.CreateNotificationMessage(ctx, deliveryMsg); err != nil {
		t.Fatalf("CreateNotificationMessage(delivery) error = %v", err)
	}

	sqsDelivery := &integrationNotifySQSClient{
		outputs: []*sqs.ReceiveMessageOutput{{
			Messages: []sqstypes.Message{{
				MessageId:     awsv2.String("sqs_delivery_1"),
				ReceiptHandle: awsv2.String("rh_delivery_1"),
				Body:          awsv2.String(mustSESFeedbackSNSBody(t, "sns_delivery_1", "Delivery", deliveryMsg.ID, projectID)),
			}},
		}},
	}
	consumer := scheduler.NewNotifySESFeedbackConsumerWithClient(st, sqsDelivery, scheduler.NotifySESFeedbackConsumerConfig{QueueURL: "queue-url", PollInterval: 10 * time.Millisecond})
	consumer.Run(mustTimedContext(t, ctx, 200*time.Millisecond))

	updatedDelivery, err := st.GetNotificationMessage(ctx, deliveryMsg.ID, projectID)
	if err != nil {
		t.Fatalf("GetNotificationMessage(delivery) error = %v", err)
	}
	if updatedDelivery.Status != domain.NotifyMessageStatusDelivered {
		t.Fatalf("delivery status = %s, want %s", updatedDelivery.Status, domain.NotifyMessageStatusDelivered)
	}
	if updatedDelivery.DeliveredAt == nil {
		t.Fatal("delivery DeliveredAt is nil, want non-nil")
	}

	complaintMsg := &domain.NotificationMessage{
		ProjectID:       projectID,
		RecipientType:   domain.NotifyRecipientTypeSubscriber,
		RecipientID:     subscriber.ID,
		Channel:         "email",
		Status:          domain.NotifyMessageStatusProcessing,
		Attempts:        1,
		RenderedContent: []byte(`{"subject":"Complaint check","text_body":"ok"}`),
	}
	if err := st.CreateNotificationMessage(ctx, complaintMsg); err != nil {
		t.Fatalf("CreateNotificationMessage(complaint) error = %v", err)
	}

	sqsComplaint := &integrationNotifySQSClient{
		outputs: []*sqs.ReceiveMessageOutput{{
			Messages: []sqstypes.Message{{
				MessageId:     awsv2.String("sqs_complaint_1"),
				ReceiptHandle: awsv2.String("rh_complaint_1"),
				Body:          awsv2.String(mustSESFeedbackSNSBody(t, "sns_complaint_1", "Complaint", complaintMsg.ID, projectID)),
			}},
		}},
	}
	consumer = scheduler.NewNotifySESFeedbackConsumerWithClient(st, sqsComplaint, scheduler.NotifySESFeedbackConsumerConfig{QueueURL: "queue-url", PollInterval: 10 * time.Millisecond})
	consumer.Run(mustTimedContext(t, ctx, 200*time.Millisecond))

	updatedComplaint, err := st.GetNotificationMessage(ctx, complaintMsg.ID, projectID)
	if err != nil {
		t.Fatalf("GetNotificationMessage(complaint) error = %v", err)
	}
	if updatedComplaint.Status != domain.NotifyMessageStatusFailed {
		t.Fatalf("complaint status = %s, want %s", updatedComplaint.Status, domain.NotifyMessageStatusFailed)
	}
	if updatedComplaint.SuppressionReason != "provider_callback:ses.complaint" {
		t.Fatalf("complaint suppression_reason = %q, want provider_callback:ses.complaint", updatedComplaint.SuppressionReason)
	}
}

func mustTimedContext(t *testing.T, parent context.Context, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, timeout)
	t.Cleanup(cancel)
	return ctx
}

func mustSESFeedbackSNSBody(t *testing.T, callbackID, eventType, straitMessageID, projectID string) string {
	t.Helper()
	eventPayload, err := json.Marshal(map[string]any{
		"eventType": eventType,
		"mail": map[string]any{
			"messageId": "ses_msg_1",
			"timestamp": "2026-04-06T10:00:00Z",
			"tags": map[string]any{
				"strait_message_id": []string{straitMessageID},
				"strait_project_id": []string{projectID},
			},
		},
		"bounce": map[string]any{"timestamp": "2026-04-06T10:01:00Z"},
	})
	if err != nil {
		t.Fatalf("marshal SES event payload: %v", err)
	}

	snsEnvelope, err := json.Marshal(map[string]any{
		"Type":      "Notification",
		"MessageId": callbackID,
		"Message":   string(eventPayload),
	})
	if err != nil {
		t.Fatalf("marshal SNS envelope: %v", err)
	}
	return string(snsEnvelope)
}
