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
	ClaimDueNotificationBatches(ctx context.Context, limit int) ([]domain.NotificationBatch, error)
	MarkNotificationBatchSent(ctx context.Context, id, projectID string, sentAt time.Time) error
	RequeueNotificationBatch(ctx context.Context, id, projectID string, windowEnd time.Time) error
	MarkNotificationBatchFailed(ctx context.Context, id, projectID string) error
	CreateNotificationMessage(ctx context.Context, msg *domain.NotificationMessage) error
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
	d.pollDueBatches(ctx)
	d.pollDueMessages(ctx)
}

func (d *NotifyDispatcher) pollDueBatches(ctx context.Context) {
	batches, err := d.store.ClaimDueNotificationBatches(ctx, d.batchSize)
	if err != nil {
		d.logger.Error("notify dispatcher: claim due batches", "error", err)
		return
	}

	for _, batch := range batches {
		if err := d.processBatch(ctx, batch); err != nil {
			d.logger.Warn("notify dispatcher: process batch failed", "batch_id", batch.ID, "project_id", batch.ProjectID, "error", err)
		}
	}
}

func (d *NotifyDispatcher) pollDueMessages(ctx context.Context) {
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

func (d *NotifyDispatcher) processBatch(ctx context.Context, batch domain.NotificationBatch) error {
	sub, err := d.store.GetNotifySubscriber(ctx, batch.RecipientID, batch.ProjectID)
	if err != nil {
		_ = d.store.MarkNotificationBatchFailed(ctx, batch.ID, batch.ProjectID)
		return fmt.Errorf("resolve batch subscriber: %w", err)
	}

	payload := buildDigestPayload(batch)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		d.requeueBatch(ctx, batch)
		return fmt.Errorf("marshal digest payload: %w", err)
	}

	msg := &domain.NotificationMessage{
		ProjectID:       batch.ProjectID,
		RecipientType:   batch.RecipientType,
		RecipientID:     batch.RecipientID,
		TenantID:        sub.TenantID,
		Channel:         batch.Channel,
		RenderedContent: payloadBytes,
		Status:          domain.NotifyMessageStatusProcessing,
		Attempts:        1,
		BatchID:         batch.ID,
	}
	if err := d.store.CreateNotificationMessage(ctx, msg); err != nil {
		d.requeueBatch(ctx, batch)
		return fmt.Errorf("create digest message: %w", err)
	}

	if err := d.deliverByChannel(ctx, msg, sub, payload); err != nil {
		d.failMessage(ctx, *msg, err)
		d.requeueBatch(ctx, batch)
		return err
	}

	now := time.Now().UTC()
	if err := d.store.UpdateNotificationMessageStatus(ctx, msg.ID, msg.ProjectID, domain.NotifyMessageStatusProcessing, domain.NotifyMessageStatusDelivered, map[string]any{"delivered_at": now}); err != nil {
		d.requeueBatch(ctx, batch)
		return fmt.Errorf("mark digest message delivered: %w", err)
	}
	if err := d.store.MarkNotificationBatchSent(ctx, batch.ID, batch.ProjectID, now); err != nil {
		return fmt.Errorf("mark batch sent: %w", err)
	}

	return nil
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

	if err := d.deliverByChannel(ctx, &msg, sub, payload); err != nil {
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

func (d *NotifyDispatcher) deliverByChannel(ctx context.Context, msg *domain.NotificationMessage, sub *domain.NotifySubscriber, payload map[string]any) error {
	switch msg.Channel {
	case "inbox":
		return d.deliverInbox(ctx, *msg, sub, payload)
	case "email":
		return d.deliverEmail(ctx, *msg, sub, payload)
	default:
		return fmt.Errorf("unsupported channel: %s", msg.Channel)
	}
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

func (d *NotifyDispatcher) requeueBatch(ctx context.Context, batch domain.NotificationBatch) {
	nextWindow := time.Now().UTC().Add(5 * time.Minute)
	if err := d.store.RequeueNotificationBatch(ctx, batch.ID, batch.ProjectID, nextWindow); err != nil {
		d.logger.Warn("notify dispatcher: requeue batch failed", "batch_id", batch.ID, "error", err)
		if markErr := d.store.MarkNotificationBatchFailed(ctx, batch.ID, batch.ProjectID); markErr != nil {
			d.logger.Warn("notify dispatcher: mark batch failed", "batch_id", batch.ID, "error", markErr)
		}
	}
}

func buildDigestPayload(batch domain.NotificationBatch) map[string]any {
	eventPayloads := extractDigestEventPayloads(batch.Events)
	count := batch.EventCount
	if count <= 0 {
		count = len(eventPayloads)
	}

	titles := collectDigestTitles(eventPayloads)
	summary := "Open your inbox to review updates."
	if len(titles) > 0 {
		summary = strings.Join(titles, "\n")
	}

	subject := fmt.Sprintf("You have %d new notifications", count)
	if count == 1 {
		subject = "You have 1 new notification"
	}

	var htmlBuilder strings.Builder
	htmlBuilder.WriteString("<p>")
	htmlBuilder.WriteString(subject)
	htmlBuilder.WriteString("</p><ul>")
	for _, title := range titles {
		htmlBuilder.WriteString("<li>")
		htmlBuilder.WriteString(title)
		htmlBuilder.WriteString("</li>")
	}
	htmlBuilder.WriteString("</ul>")

	return map[string]any{
		"title":     subject,
		"body":      summary,
		"priority":  "normal",
		"subject":   subject,
		"html_body": htmlBuilder.String(),
		"text_body": subject + "\n\n" + summary,
	}
}

func extractDigestEventPayloads(events json.RawMessage) []map[string]any {
	if len(events) == 0 {
		return nil
	}

	var rows []map[string]any
	if err := json.Unmarshal(events, &rows); err != nil {
		return nil
	}

	payloads := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		payload, ok := row["channel_payload"].(map[string]any)
		if ok {
			payloads = append(payloads, payload)
		}
	}
	return payloads
}

func collectDigestTitles(events []map[string]any) []string {
	if len(events) == 0 {
		return nil
	}

	titles := make([]string, 0, len(events))
	for _, event := range events {
		if title, ok := event["title"].(string); ok && title != "" {
			titles = append(titles, title)
			continue
		}
		if subject, ok := event["subject"].(string); ok && subject != "" {
			titles = append(titles, subject)
		}
	}
	if len(titles) > 3 {
		return titles[:3]
	}
	return titles
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
