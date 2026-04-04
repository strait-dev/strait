package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"strait/internal/domain"

	"github.com/resend/resend-go/v2"
)

type notifyDispatcherStore interface {
	ClaimDueScheduledNotificationMessages(ctx context.Context, limit int) ([]domain.NotificationMessage, error)
	UpdateNotificationMessageStatus(ctx context.Context, id, projectID, fromStatus, toStatus string, fields map[string]any) error
	GetNotifySubscriber(ctx context.Context, id, projectID string) (*domain.NotifySubscriber, error)
	CreateInboxItem(ctx context.Context, item *domain.InboxItem) error
	GetNotificationProvider(ctx context.Context, id, projectID string) (*domain.NotificationProvider, error)
}

type NotifyDispatcher struct {
	store        notifyDispatcherStore
	interval     time.Duration
	batchSize    int
	resendAPIKey string
	resendFrom   string
	logger       *slog.Logger
}

func NewNotifyDispatcher(s notifyDispatcherStore, interval time.Duration, resendAPIKey, resendFrom string) *NotifyDispatcher {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if resendFrom == "" {
		resendFrom = "noreply@strait.dev"
	}

	return &NotifyDispatcher{
		store:        s,
		interval:     interval,
		batchSize:    100,
		resendAPIKey: resendAPIKey,
		resendFrom:   resendFrom,
		logger:       slog.Default(),
	}
}

func (d *NotifyDispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.poll(ctx)
		}
	}
}

func (d *NotifyDispatcher) poll(ctx context.Context) {
	messages, err := d.store.ClaimDueScheduledNotificationMessages(ctx, d.batchSize)
	if err != nil {
		d.logger.Error("notify dispatcher: claim due messages", "error", err)
		return
	}

	for _, msg := range messages {
		if err := d.processMessage(ctx, msg); err != nil {
			d.logger.Warn("notify dispatcher: process message failed", "message_id", msg.ID, "project_id", msg.ProjectID, "error", err)
		}
	}
}

func (d *NotifyDispatcher) processMessage(ctx context.Context, msg domain.NotificationMessage) error {
	sub, err := d.store.GetNotifySubscriber(ctx, msg.RecipientID, msg.ProjectID)
	if err != nil {
		d.failMessage(ctx, msg, fmt.Errorf("resolve subscriber: %w", err))
		return err
	}

	payload := map[string]any{}
	if len(msg.RenderedContent) > 0 {
		if err := json.Unmarshal(msg.RenderedContent, &payload); err != nil {
			d.failMessage(ctx, msg, fmt.Errorf("decode rendered content: %w", err))
			return err
		}
	}

	switch msg.Channel {
	case "inbox":
		if err := d.deliverInbox(ctx, msg, sub, payload); err != nil {
			d.failMessage(ctx, msg, err)
			return err
		}
	case "email":
		if err := d.deliverEmail(ctx, msg, sub, payload); err != nil {
			d.failMessage(ctx, msg, err)
			return err
		}
	default:
		err := fmt.Errorf("unsupported channel: %s", msg.Channel)
		d.failMessage(ctx, msg, err)
		return err
	}

	now := time.Now().UTC()
	if err := d.store.UpdateNotificationMessageStatus(ctx, msg.ID, msg.ProjectID, domain.NotifyMessageStatusProcessing, domain.NotifyMessageStatusDelivered, map[string]any{
		"delivered_at": now,
	}); err != nil {
		return fmt.Errorf("mark message delivered: %w", err)
	}

	return nil
}

func (d *NotifyDispatcher) deliverInbox(ctx context.Context, msg domain.NotificationMessage, sub *domain.NotifySubscriber, payload map[string]any) error {
	title, _ := payload["title"].(string)
	if title == "" {
		title = "Notification"
	}
	body, _ := payload["body"].(string)
	avatar, _ := payload["avatar"].(string)
	priority, _ := payload["priority"].(string)
	if priority == "" {
		priority = "normal"
	}

	actionsRaw := []byte("[]")
	if actions, ok := payload["actions"]; ok {
		encoded, err := json.Marshal(actions)
		if err != nil {
			return fmt.Errorf("encode inbox actions: %w", err)
		}
		actionsRaw = encoded
	}

	item := &domain.InboxItem{
		RecipientType: domain.NotifyRecipientTypeSubscriber,
		RecipientID:   sub.ID,
		ProjectID:     msg.ProjectID,
		TenantID:      sub.TenantID,
		WorkflowRunID: msg.WorkflowRunID,
		CategoryKey:   msg.CategoryKey,
		Title:         title,
		Body:          body,
		Avatar:        avatar,
		Priority:      priority,
		State:         domain.NotifyInboxStateUnread,
		Actions:       actionsRaw,
	}

	if err := d.store.CreateInboxItem(ctx, item); err != nil {
		return fmt.Errorf("create inbox item: %w", err)
	}

	return nil
}

func (d *NotifyDispatcher) deliverEmail(ctx context.Context, msg domain.NotificationMessage, sub *domain.NotifySubscriber, payload map[string]any) error {
	if sub.Email == "" {
		return fmt.Errorf("subscriber email is required")
	}

	subject, _ := payload["subject"].(string)
	htmlBody, _ := payload["html_body"].(string)
	textBody, _ := payload["text_body"].(string)
	if subject == "" {
		return fmt.Errorf("email subject is required")
	}
	if htmlBody == "" && textBody == "" {
		return fmt.Errorf("email body is required")
	}

	providerName := "resend"
	apiKey := d.resendAPIKey
	fromEmail := d.resendFrom

	if msg.ProviderID != "" {
		provider, err := d.store.GetNotificationProvider(ctx, msg.ProviderID, msg.ProjectID)
		if err != nil {
			return fmt.Errorf("resolve provider: %w", err)
		}
		providerName = provider.Provider

		cfg := struct {
			APIKey    string `json:"api_key"`
			FromEmail string `json:"from_email"`
		}{}
		if err := json.Unmarshal(provider.ConfigEnc, &cfg); err != nil {
			return fmt.Errorf("decode provider config: %w", err)
		}
		if cfg.APIKey != "" {
			apiKey = cfg.APIKey
		}
		if cfg.FromEmail != "" {
			fromEmail = cfg.FromEmail
		}
	}

	if strings.ToLower(providerName) != "resend" {
		return fmt.Errorf("unsupported email provider: %s", providerName)
	}
	if apiKey == "" {
		return fmt.Errorf("resend api key is required")
	}

	client := resend.NewClient(apiKey)
	_, err := client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From:    fromEmail,
		To:      []string{sub.Email},
		Subject: subject,
		Html:    htmlBody,
		Text:    textBody,
	})
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}

	return nil
}

func (d *NotifyDispatcher) failMessage(ctx context.Context, msg domain.NotificationMessage, reason error) {
	if reason == nil {
		return
	}
	if err := d.store.UpdateNotificationMessageStatus(ctx, msg.ID, msg.ProjectID, domain.NotifyMessageStatusProcessing, domain.NotifyMessageStatusFailed, map[string]any{
		"suppression_reason": reason.Error(),
	}); err != nil {
		d.logger.Warn("notify dispatcher: mark failed", "message_id", msg.ID, "error", err)
	}
}
