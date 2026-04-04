package scheduler

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
)

type notifyDispatcherStoreMock struct {
	claimFunc        func(context.Context, int) ([]domain.NotificationMessage, error)
	updateStatusFunc func(context.Context, string, string, string, string, map[string]any) error
	getSubFunc       func(context.Context, string, string) (*domain.NotifySubscriber, error)
	createInboxFunc  func(context.Context, *domain.InboxItem) error
	getProviderFunc  func(context.Context, string, string) (*domain.NotificationProvider, error)
}

func (m *notifyDispatcherStoreMock) ClaimDueScheduledNotificationMessages(ctx context.Context, limit int) ([]domain.NotificationMessage, error) {
	if m.claimFunc == nil {
		return nil, nil
	}
	return m.claimFunc(ctx, limit)
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
