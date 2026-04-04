package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
)

type notifyDispatcherStoreMock struct {
	claimFunc           func(context.Context, int) ([]domain.NotificationMessage, error)
	claimBatchFunc      func(context.Context, int) ([]domain.NotificationBatch, error)
	markBatchSentFunc   func(context.Context, string, string, time.Time) error
	requeueBatchFunc    func(context.Context, string, string, time.Time) error
	markBatchFailedFunc func(context.Context, string, string) error
	createMessageFunc   func(context.Context, *domain.NotificationMessage) error
	updateStatusFunc    func(context.Context, string, string, string, string, map[string]any) error
	getSubFunc          func(context.Context, string, string) (*domain.NotifySubscriber, error)
	createInboxFunc     func(context.Context, *domain.InboxItem) error
	getProviderFunc     func(context.Context, string, string) (*domain.NotificationProvider, error)
}

func (m *notifyDispatcherStoreMock) ClaimDueScheduledNotificationMessages(ctx context.Context, limit int) ([]domain.NotificationMessage, error) {
	if m.claimFunc == nil {
		return nil, nil
	}
	return m.claimFunc(ctx, limit)
}

func (m *notifyDispatcherStoreMock) ClaimDueNotificationBatches(ctx context.Context, limit int) ([]domain.NotificationBatch, error) {
	if m.claimBatchFunc == nil {
		return nil, nil
	}
	return m.claimBatchFunc(ctx, limit)
}

func (m *notifyDispatcherStoreMock) MarkNotificationBatchSent(ctx context.Context, id, projectID string, sentAt time.Time) error {
	if m.markBatchSentFunc == nil {
		return nil
	}
	return m.markBatchSentFunc(ctx, id, projectID, sentAt)
}

func (m *notifyDispatcherStoreMock) RequeueNotificationBatch(ctx context.Context, id, projectID string, windowEnd time.Time) error {
	if m.requeueBatchFunc == nil {
		return nil
	}
	return m.requeueBatchFunc(ctx, id, projectID, windowEnd)
}

func (m *notifyDispatcherStoreMock) MarkNotificationBatchFailed(ctx context.Context, id, projectID string) error {
	if m.markBatchFailedFunc == nil {
		return nil
	}
	return m.markBatchFailedFunc(ctx, id, projectID)
}

func (m *notifyDispatcherStoreMock) CreateNotificationMessage(ctx context.Context, msg *domain.NotificationMessage) error {
	if m.createMessageFunc == nil {
		return nil
	}
	return m.createMessageFunc(ctx, msg)
}

func (m *notifyDispatcherStoreMock) UpdateNotificationMessageStatus(ctx context.Context, id, projectID, fromStatus, toStatus string, fields map[string]any) error {
	if m.updateStatusFunc == nil {
		return nil
	}
	return m.updateStatusFunc(ctx, id, projectID, fromStatus, toStatus, fields)
}

func (m *notifyDispatcherStoreMock) GetNotifySubscriber(ctx context.Context, id, projectID string) (*domain.NotifySubscriber, error) {
	if m.getSubFunc == nil {
		return nil, errors.New("not implemented")
	}
	return m.getSubFunc(ctx, id, projectID)
}

func (m *notifyDispatcherStoreMock) CreateInboxItem(ctx context.Context, item *domain.InboxItem) error {
	if m.createInboxFunc == nil {
		return nil
	}
	return m.createInboxFunc(ctx, item)
}

func (m *notifyDispatcherStoreMock) GetNotificationProvider(ctx context.Context, id, projectID string) (*domain.NotificationProvider, error) {
	if m.getProviderFunc == nil {
		return nil, errors.New("not implemented")
	}
	return m.getProviderFunc(ctx, id, projectID)
}

func TestNotifyDispatcherPoll_InboxDelivered(t *testing.T) {
	t.Parallel()

	msg := domain.NotificationMessage{
		ID:              "msg_1",
		ProjectID:       "proj_1",
		RecipientType:   domain.NotifyRecipientTypeSubscriber,
		RecipientID:     "sub_1",
		Channel:         "inbox",
		Status:          domain.NotifyMessageStatusProcessing,
		RenderedContent: []byte(`{"title":"Hello","body":"World"}`),
	}

	var inboxCreated bool
	var delivered bool
	st := &notifyDispatcherStoreMock{
		claimFunc: func(context.Context, int) ([]domain.NotificationMessage, error) {
			return []domain.NotificationMessage{msg}, nil
		},
		getSubFunc: func(context.Context, string, string) (*domain.NotifySubscriber, error) {
			return &domain.NotifySubscriber{ID: "sub_1", ProjectID: "proj_1", TenantID: "tenant_1"}, nil
		},
		createInboxFunc: func(_ context.Context, item *domain.InboxItem) error {
			inboxCreated = true
			if item.Title != "Hello" {
				t.Fatalf("CreateInboxItem title = %q, want Hello", item.Title)
			}
			return nil
		},
		updateStatusFunc: func(_ context.Context, id, projectID, fromStatus, toStatus string, _ map[string]any) error {
			if id != msg.ID || projectID != msg.ProjectID {
				t.Fatalf("UpdateNotificationMessageStatus args = (%s,%s), want (%s,%s)", id, projectID, msg.ID, msg.ProjectID)
			}
			if fromStatus == domain.NotifyMessageStatusProcessing && toStatus == domain.NotifyMessageStatusDelivered {
				delivered = true
			}
			return nil
		},
	}

	d := NewNotifyDispatcher(st, 0, "", "")
	d.poll(context.Background())

	if !inboxCreated {
		t.Fatal("expected inbox item creation")
	}
	if !delivered {
		t.Fatal("expected delivered status update")
	}
}

func TestNotifyDispatcherPoll_DigestBatchDelivered(t *testing.T) {
	t.Parallel()

	batch := domain.NotificationBatch{
		ID:            "batch_1",
		ProjectID:     "proj_1",
		RecipientType: domain.NotifyRecipientTypeSubscriber,
		RecipientID:   "sub_1",
		Channel:       "inbox",
		Status:        domain.NotifyBatchStatusProcessing,
		EventCount:    2,
		Events:        []byte(`[{"channel_payload":{"title":"Event A"}},{"channel_payload":{"title":"Event B"}}]`),
	}

	var messageCreated bool
	var batchSent bool
	st := &notifyDispatcherStoreMock{
		claimBatchFunc: func(context.Context, int) ([]domain.NotificationBatch, error) {
			return []domain.NotificationBatch{batch}, nil
		},
		getSubFunc: func(context.Context, string, string) (*domain.NotifySubscriber, error) {
			return &domain.NotifySubscriber{ID: "sub_1", ProjectID: "proj_1", TenantID: "tenant_1"}, nil
		},
		createMessageFunc: func(_ context.Context, msg *domain.NotificationMessage) error {
			messageCreated = true
			msg.ID = "msg_digest_1"
			return nil
		},
		createInboxFunc: func(_ context.Context, item *domain.InboxItem) error {
			if item.Title == "" {
				t.Fatal("expected digest inbox title")
			}
			return nil
		},
		updateStatusFunc: func(_ context.Context, _, _ string, fromStatus, toStatus string, _ map[string]any) error {
			if fromStatus == domain.NotifyMessageStatusProcessing && toStatus == domain.NotifyMessageStatusDelivered {
				return nil
			}
			return nil
		},
		markBatchSentFunc: func(_ context.Context, id, projectID string, _ time.Time) error {
			if id != "batch_1" || projectID != "proj_1" {
				t.Fatalf("MarkNotificationBatchSent args = (%s,%s), want (batch_1,proj_1)", id, projectID)
			}
			batchSent = true
			return nil
		},
	}

	d := NewNotifyDispatcher(st, 0, "", "")
	d.poll(context.Background())

	if !messageCreated {
		t.Fatal("expected digest notification message creation")
	}
	if !batchSent {
		t.Fatal("expected batch sent status update")
	}
}

func TestNotifyDispatcherPoll_UnsupportedChannelFails(t *testing.T) {
	t.Parallel()

	msg := domain.NotificationMessage{
		ID:            "msg_2",
		ProjectID:     "proj_1",
		RecipientType: domain.NotifyRecipientTypeSubscriber,
		RecipientID:   "sub_1",
		Channel:       "sms",
		Status:        domain.NotifyMessageStatusProcessing,
	}

	var failed bool
	st := &notifyDispatcherStoreMock{
		claimFunc: func(context.Context, int) ([]domain.NotificationMessage, error) {
			return []domain.NotificationMessage{msg}, nil
		},
		getSubFunc: func(context.Context, string, string) (*domain.NotifySubscriber, error) {
			return &domain.NotifySubscriber{ID: "sub_1", ProjectID: "proj_1"}, nil
		},
		updateStatusFunc: func(_ context.Context, _, _ string, fromStatus, toStatus string, fields map[string]any) error {
			if fromStatus == domain.NotifyMessageStatusProcessing && toStatus == domain.NotifyMessageStatusFailed {
				failed = true
				if fields["suppression_reason"] == "" {
					t.Fatal("expected suppression_reason to be set")
				}
			}
			return nil
		},
	}

	d := NewNotifyDispatcher(st, 0, "", "")
	d.poll(context.Background())

	if !failed {
		t.Fatal("expected failed status update")
	}
}
