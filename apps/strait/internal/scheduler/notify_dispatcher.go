package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/telemetry"

	"github.com/resend/resend-go/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	notifyEscalationDefaultInterval = 15 * time.Minute
)

type notifyDispatcherStore interface {
	ClaimDueScheduledNotificationMessages(ctx context.Context, limit int) ([]domain.NotificationMessage, error)
	ClaimDueNotificationBatches(ctx context.Context, limit int) ([]domain.NotificationBatch, error)
	MarkNotificationBatchSent(ctx context.Context, id, projectID string, sentAt time.Time) error
	RequeueNotificationBatch(ctx context.Context, id, projectID string, windowEnd time.Time) error
	MarkNotificationBatchFailed(ctx context.Context, id, projectID string) error
	ClaimDueEscalationStates(ctx context.Context, limit int) ([]domain.EscalationState, error)
	AdvanceEscalationState(ctx context.Context, id, projectID string, currentTier int, nextEscalationAt *time.Time, status string) error
	GetWorkflowStepApprovalByStepRunID(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error)
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error
	CreateNotificationMessage(ctx context.Context, msg *domain.NotificationMessage) error
	UpdateNotificationMessageStatus(ctx context.Context, id, projectID, fromStatus, toStatus string, fields map[string]any) error
	GetNotifySubscriber(ctx context.Context, id, projectID string) (*domain.NotifySubscriber, error)
	CreateInboxItem(ctx context.Context, item *domain.InboxItem) error
	GetNotificationProvider(ctx context.Context, id, projectID string) (*domain.NotificationProvider, error)
	ListNotificationProviders(ctx context.Context, projectID, channel string) ([]domain.NotificationProvider, error)
}

type notifyPolicyResolver interface {
	ResolveNotifyPolicyOverride(ctx context.Context, projectID, stepRunID, categoryKey, channel string) (*domain.NotifyPolicyOverride, error)
}

type NotifyDispatcher struct {
	store                 notifyDispatcherStore
	interval              time.Duration
	batchSize             int
	resendAPIKey          string
	resendFrom            string
	maxAttempts           int
	retryBaseDelay        time.Duration
	retryMaxDelay         time.Duration
	digestMaxItems        int
	digestMaxTitleChars   int
	escalationMinInterval time.Duration
	logger                *slog.Logger
	metrics               *telemetry.Metrics
}

func NewNotifyDispatcher(s notifyDispatcherStore, interval time.Duration, resendAPIKey, resendFrom string) *NotifyDispatcher {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if resendFrom == "" {
		resendFrom = "noreply@strait.dev"
	}

	return &NotifyDispatcher{
		store:                 s,
		interval:              interval,
		batchSize:             100,
		resendAPIKey:          resendAPIKey,
		resendFrom:            resendFrom,
		maxAttempts:           5,
		retryBaseDelay:        30 * time.Second,
		retryMaxDelay:         15 * time.Minute,
		digestMaxItems:        50,
		digestMaxTitleChars:   120,
		escalationMinInterval: notifyEscalationDefaultInterval,
		logger:                slog.Default(),
	}
}

func (d *NotifyDispatcher) WithMetrics(m *telemetry.Metrics) *NotifyDispatcher {
	d.metrics = m
	return d
}

func (d *NotifyDispatcher) WithRetryPolicy(maxAttempts int, baseDelay, maxDelay time.Duration) *NotifyDispatcher {
	if maxAttempts > 0 {
		d.maxAttempts = maxAttempts
	}
	if baseDelay > 0 {
		d.retryBaseDelay = baseDelay
	}
	if maxDelay > 0 {
		d.retryMaxDelay = maxDelay
	}
	if d.retryMaxDelay < d.retryBaseDelay {
		d.retryMaxDelay = d.retryBaseDelay
	}
	return d
}

func (d *NotifyDispatcher) WithDigestLimits(maxItems, maxTitleChars int) *NotifyDispatcher {
	if maxItems > 0 {
		d.digestMaxItems = maxItems
	}
	if maxTitleChars > 0 {
		d.digestMaxTitleChars = maxTitleChars
	}
	return d
}

func (d *NotifyDispatcher) WithEscalationMinInterval(minInterval time.Duration) *NotifyDispatcher {
	if minInterval > 0 {
		d.escalationMinInterval = minInterval
	}
	return d
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
	d.pollDueEscalations(ctx)
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
	if d.metrics != nil {
		d.metrics.NotifyScheduledBacklog.Record(ctx, int64(len(messages)))
	}

	for _, msg := range messages {
		if err := d.processMessage(ctx, msg); err != nil {
			d.logger.Warn("notify dispatcher: process message failed", "message_id", msg.ID, "project_id", msg.ProjectID, "error", err)
		}
	}
}

func (d *NotifyDispatcher) pollDueEscalations(ctx context.Context) {
	states, err := d.store.ClaimDueEscalationStates(ctx, d.batchSize)
	if err != nil {
		d.logger.Error("notify dispatcher: claim due escalations", "error", err)
		return
	}

	for _, state := range states {
		if err := d.processEscalation(ctx, state); err != nil {
			d.logger.Warn("notify dispatcher: process escalation failed", "escalation_id", state.ID, "project_id", state.ProjectID, "error", err)
		}
	}
}

func (d *NotifyDispatcher) processEscalation(ctx context.Context, state domain.EscalationState) error {
	approval, err := d.store.GetWorkflowStepApprovalByStepRunID(ctx, state.StepRunID)
	if err != nil {
		d.requeueEscalation(ctx, state)
		return fmt.Errorf("resolve workflow step approval: %w", err)
	}
	if approval == nil || approval.Status != domain.ApprovalStatusPending {
		if err := d.store.AdvanceEscalationState(ctx, state.ID, state.ProjectID, state.CurrentTier, nil, domain.NotifyEscalationStatusCompleted); err != nil {
			return fmt.Errorf("complete escalation state: %w", err)
		}
		d.recordEscalationTransition(ctx, domain.NotifyEscalationStatusCompleted)
		return nil
	}

	wfRun, err := d.store.GetWorkflowRun(ctx, state.WorkflowRunID)
	if err != nil || wfRun == nil {
		d.requeueEscalation(ctx, state)
		if err != nil {
			return fmt.Errorf("resolve workflow run: %w", err)
		}
		return fmt.Errorf("resolve workflow run: not found")
	}

	channels, err := d.store.ListEnabledNotificationChannels(ctx, state.ProjectID)
	if err != nil {
		d.requeueEscalation(ctx, state)
		return fmt.Errorf("list notification channels: %w", err)
	}

	nextTier := state.CurrentTier + 1
	payload, err := json.Marshal(map[string]any{
		"workflow_run_id": state.WorkflowRunID,
		"workflow_id":     wfRun.WorkflowID,
		"step_run_id":     state.StepRunID,
		"escalation_tier": nextTier,
		"total_tiers":     state.TotalTiers,
		"approval_id":     approval.ID,
	})
	if err != nil {
		d.requeueEscalation(ctx, state)
		return fmt.Errorf("marshal escalation payload: %w", err)
	}

	for _, channel := range channels {
		delivery := &domain.NotificationDelivery{
			ChannelID:   channel.ID,
			ProjectID:   state.ProjectID,
			EventType:   domain.NotificationEventApprovalReminder,
			Payload:     payload,
			Status:      "pending",
			MaxAttempts: d.effectiveDeliveryMaxAttempts(ctx, state.ProjectID, state.StepRunID, channel.ChannelType, 3),
		}
		if err := d.store.CreateNotificationDelivery(ctx, delivery); err != nil {
			d.requeueEscalation(ctx, state)
			return fmt.Errorf("create escalation delivery: %w", err)
		}
	}

	status := domain.NotifyEscalationStatusActive
	var nextAt *time.Time
	if nextTier >= state.TotalTiers {
		status = domain.NotifyEscalationStatusCompleted
	} else {
		minInterval := d.effectiveEscalationMinInterval(ctx, state)
		t := time.Now().UTC().Add(d.computeEscalationInterval(approval, nextTier, state.TotalTiers, minInterval))
		nextAt = &t
	}

	if err := d.store.AdvanceEscalationState(ctx, state.ID, state.ProjectID, nextTier, nextAt, status); err != nil {
		return fmt.Errorf("advance escalation state: %w", err)
	}
	d.recordEscalationTransition(ctx, status)

	return nil
}

func (d *NotifyDispatcher) computeEscalationInterval(approval *domain.WorkflowStepApproval, nextTier, totalTiers int, minInterval time.Duration) time.Duration {
	if minInterval <= 0 {
		minInterval = d.escalationMinInterval
	}
	if approval == nil || approval.ExpiresAt == nil || totalTiers <= nextTier {
		return minInterval
	}

	remaining := time.Until(*approval.ExpiresAt)
	if remaining <= 0 {
		return minInterval
	}

	remainingEscalations := totalTiers - nextTier
	if remainingEscalations <= 0 {
		return minInterval
	}

	interval := remaining / time.Duration(remainingEscalations+1)
	if interval < minInterval {
		return minInterval
	}
	return interval
}

func (d *NotifyDispatcher) processBatch(ctx context.Context, batch domain.NotificationBatch) error {
	sub, err := d.store.GetNotifySubscriber(ctx, batch.RecipientID, batch.ProjectID)
	if err != nil {
		_ = d.store.MarkNotificationBatchFailed(ctx, batch.ID, batch.ProjectID)
		d.recordDigestFailure(ctx, "resolve_subscriber")
		return fmt.Errorf("resolve batch subscriber: %w", err)
	}

	payload := d.buildDigestPayload(batch)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		d.requeueBatch(ctx, batch, "marshal_payload")
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
		d.requeueBatch(ctx, batch, "create_message")
		return fmt.Errorf("create digest message: %w", err)
	}

	if err := d.deliverByChannel(ctx, msg, sub, payload); err != nil {
		d.failMessage(ctx, *msg, err)
		d.requeueBatch(ctx, batch, "deliver_channel")
		return err
	}

	now := time.Now().UTC()
	if err := d.store.UpdateNotificationMessageStatus(ctx, msg.ID, msg.ProjectID, domain.NotifyMessageStatusProcessing, domain.NotifyMessageStatusDelivered, map[string]any{"delivered_at": now}); err != nil {
		d.requeueBatch(ctx, batch, "mark_message_delivered")
		return fmt.Errorf("mark digest message delivered: %w", err)
	}
	if err := d.store.MarkNotificationBatchSent(ctx, batch.ID, batch.ProjectID, now); err != nil {
		d.recordDigestFailure(ctx, "mark_batch_sent")
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
		if d.retryMessage(ctx, msg, err) {
			return nil
		}
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
		MessageID:     msg.ID,
		CategoryKey:   msg.CategoryKey,
		Title:         title,
		Body:          body,
		Avatar:        avatar,
		Priority:      priority,
		State:         domain.NotifyInboxStateUnread,
		Actions:       actionsRaw,
	}

	if err := d.store.CreateInboxItem(ctx, item); err != nil {
		if errors.Is(err, store.ErrInboxItemAlreadyExists) {
			return nil
		}
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

	attempts, err := d.resolveEmailProviderAttempts(ctx, msg)
	if err != nil {
		return err
	}

	var sendErr error
	for _, attempt := range attempts {
		err := d.sendWithProvider(ctx, attempt, sub.Email, subject, htmlBody, textBody)
		if err == nil {
			return nil
		}
		d.logger.Warn("notify dispatcher: email send attempt failed",
			"message_id", msg.ID,
			"provider_id", attempt.ID,
			"provider", attempt.Provider,
			"error", err,
		)
		sendErr = errors.Join(sendErr, err)
	}

	if sendErr == nil {
		return fmt.Errorf("email delivery failed: no providers configured")
	}
	return sendErr
}

type emailProviderAttempt struct {
	ID         string
	Provider   string
	FromEmail  string
	APIKey     string
	FallbackID string
}

func (d *NotifyDispatcher) resolveEmailProviderAttempts(ctx context.Context, msg domain.NotificationMessage) ([]emailProviderAttempt, error) {
	attempts := make([]emailProviderAttempt, 0, 4)
	seen := map[string]struct{}{}

	appendConfigFallback := func() {
		if d.resendAPIKey == "" {
			return
		}
		attempts = append(attempts, emailProviderAttempt{
			Provider:  "resend",
			FromEmail: d.resendFrom,
			APIKey:    d.resendAPIKey,
		})
	}

	if msg.ProviderID != "" {
		chain, err := d.resolveProviderFallbackChain(ctx, msg.ProjectID, msg.ProviderID, seen)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, chain...)
		appendConfigFallback()
		return attempts, nil
	}

	providers, err := d.store.ListNotificationProviders(ctx, msg.ProjectID, "email")
	if err == nil {
		for _, provider := range providers {
			if !provider.IsDefault {
				continue
			}
			chain, chainErr := d.resolveProviderFallbackChain(ctx, msg.ProjectID, provider.ID, seen)
			if chainErr != nil {
				return nil, chainErr
			}
			attempts = append(attempts, chain...)
			break
		}
	}
	appendConfigFallback()
	if len(attempts) == 0 {
		return nil, fmt.Errorf("email delivery failed: no providers configured")
	}
	return attempts, nil
}

func (d *NotifyDispatcher) resolveProviderFallbackChain(ctx context.Context, projectID, providerID string, seen map[string]struct{}) ([]emailProviderAttempt, error) {
	if providerID == "" {
		return nil, nil
	}
	if _, ok := seen[providerID]; ok {
		return nil, nil
	}
	seen[providerID] = struct{}{}

	provider, err := d.store.GetNotificationProvider(ctx, providerID, projectID)
	if err != nil {
		return nil, fmt.Errorf("resolve provider: %w", err)
	}

	cfg := struct {
		APIKey    string `json:"api_key"`
		FromEmail string `json:"from_email"`
	}{}
	if len(provider.ConfigEnc) > 0 {
		if err := json.Unmarshal(provider.ConfigEnc, &cfg); err != nil {
			return nil, fmt.Errorf("decode provider config: %w", err)
		}
	}

	attempt := emailProviderAttempt{
		ID:         provider.ID,
		Provider:   provider.Provider,
		FromEmail:  cfg.FromEmail,
		APIKey:     cfg.APIKey,
		FallbackID: provider.FallbackID,
	}
	if attempt.FromEmail == "" {
		attempt.FromEmail = d.resendFrom
	}

	chain := []emailProviderAttempt{attempt}
	if provider.FallbackID == "" {
		return chain, nil
	}
	next, err := d.resolveProviderFallbackChain(ctx, projectID, provider.FallbackID, seen)
	if err != nil {
		return nil, err
	}
	return append(chain, next...), nil
}

func (d *NotifyDispatcher) sendWithProvider(ctx context.Context, attempt emailProviderAttempt, to, subject, htmlBody, textBody string) error {
	if strings.ToLower(attempt.Provider) != "resend" {
		return fmt.Errorf("unsupported email provider: %s", attempt.Provider)
	}
	if attempt.APIKey == "" {
		return fmt.Errorf("resend api key is required")
	}

	client := resend.NewClient(attempt.APIKey)
	_, err := client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From:    attempt.FromEmail,
		To:      []string{to},
		Subject: subject,
		Html:    htmlBody,
		Text:    textBody,
	})
	if err != nil {
		return fmt.Errorf("send email (%s): %w", attempt.Provider, err)
	}
	return nil
}

func (d *NotifyDispatcher) requeueBatch(ctx context.Context, batch domain.NotificationBatch, reason string) {
	d.recordDigestRequeue(ctx, reason)
	nextWindow := time.Now().UTC().Add(5 * time.Minute)
	if err := d.store.RequeueNotificationBatch(ctx, batch.ID, batch.ProjectID, nextWindow); err != nil {
		d.logger.Warn("notify dispatcher: requeue batch failed", "batch_id", batch.ID, "error", err)
		d.recordDigestFailure(ctx, "requeue_failed")
		if markErr := d.store.MarkNotificationBatchFailed(ctx, batch.ID, batch.ProjectID); markErr != nil {
			d.logger.Warn("notify dispatcher: mark batch failed", "batch_id", batch.ID, "error", markErr)
		}
	}
}

func (d *NotifyDispatcher) requeueEscalation(ctx context.Context, state domain.EscalationState) {
	d.recordEscalationStuck(ctx, "requeue")
	nextAt := time.Now().UTC().Add(5 * time.Minute)
	if err := d.store.AdvanceEscalationState(ctx, state.ID, state.ProjectID, state.CurrentTier, &nextAt, domain.NotifyEscalationStatusActive); err != nil {
		d.logger.Warn("notify dispatcher: requeue escalation failed", "escalation_id", state.ID, "error", err)
		return
	}
	d.recordEscalationTransition(ctx, domain.NotifyEscalationStatusActive)
}

func (d *NotifyDispatcher) recordDigestRequeue(ctx context.Context, reason string) {
	if d.metrics == nil {
		return
	}
	if reason == "" {
		reason = "unknown"
	}
	d.metrics.NotifyDigestRequeuesTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

func (d *NotifyDispatcher) recordDigestFailure(ctx context.Context, reason string) {
	if d.metrics == nil {
		return
	}
	if reason == "" {
		reason = "unknown"
	}
	d.metrics.NotifyDigestFailuresTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

func (d *NotifyDispatcher) recordEscalationTransition(ctx context.Context, status string) {
	if d.metrics == nil {
		return
	}
	if status == "" {
		status = "unknown"
	}
	d.metrics.NotifyEscalationTransitions.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status)))
}

func (d *NotifyDispatcher) recordEscalationStuck(ctx context.Context, reason string) {
	if d.metrics == nil {
		return
	}
	if reason == "" {
		reason = "unknown"
	}
	d.metrics.NotifyEscalationStuckStates.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

func (d *NotifyDispatcher) buildDigestPayload(batch domain.NotificationBatch) map[string]any {
	eventPayloads := extractDigestEventPayloads(batch.Events)
	count := batch.EventCount
	if count <= 0 {
		count = len(eventPayloads)
	}

	titles := d.collectDigestTitles(eventPayloads)
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

	if hidden := count - len(titles); hidden > 0 {
		fmt.Fprintf(&htmlBuilder, "<p>+%d more updates</p>", hidden)
		summary += fmt.Sprintf("\n+%d more updates", hidden)
	}

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

func (d *NotifyDispatcher) collectDigestTitles(events []map[string]any) []string {
	if len(events) == 0 {
		return nil
	}

	maxItems := d.digestMaxItems
	if maxItems <= 0 {
		maxItems = 3
	}
	maxTitleChars := d.digestMaxTitleChars
	if maxTitleChars <= 0 {
		maxTitleChars = 120
	}

	titles := make([]string, 0, minInt(len(events), maxItems))
	for _, event := range events {
		if len(titles) >= maxItems {
			break
		}
		candidate := ""
		if title, ok := event["title"].(string); ok && title != "" {
			candidate = title
		} else if subject, ok := event["subject"].(string); ok && subject != "" {
			candidate = subject
		}
		if candidate == "" {
			continue
		}
		if len(candidate) > maxTitleChars {
			candidate = strings.TrimSpace(candidate[:maxTitleChars-1]) + "…"
		}
		titles = append(titles, candidate)
	}
	return titles
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (d *NotifyDispatcher) resolvePolicyOverride(ctx context.Context, projectID, stepRunID, categoryKey, channel string) (*domain.NotifyPolicyOverride, error) {
	resolver, ok := d.store.(notifyPolicyResolver)
	if !ok {
		return nil, nil
	}

	policy, err := resolver.ResolveNotifyPolicyOverride(ctx, projectID, stepRunID, categoryKey, channel)
	if err != nil {
		if errors.Is(err, store.ErrNotifyPolicyNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return policy, nil
}

func (d *NotifyDispatcher) effectiveRetryPolicy(ctx context.Context, msg domain.NotificationMessage) (int, time.Duration, time.Duration) {
	maxAttempts := d.maxAttempts
	baseDelay := d.retryBaseDelay
	maxDelay := d.retryMaxDelay

	policy, err := d.resolvePolicyOverride(ctx, msg.ProjectID, msg.StepRunID, msg.CategoryKey, msg.Channel)
	if err != nil {
		d.logger.Warn("notify dispatcher: resolve retry policy override", "project_id", msg.ProjectID, "message_id", msg.ID, "error", err)
		return maxAttempts, baseDelay, maxDelay
	}
	if policy == nil {
		return maxAttempts, baseDelay, maxDelay
	}

	if policy.RetryMaxAttempts != nil && *policy.RetryMaxAttempts > 0 {
		maxAttempts = *policy.RetryMaxAttempts
	}
	if policy.RetryBaseDelaySecs != nil && *policy.RetryBaseDelaySecs > 0 {
		baseDelay = time.Duration(*policy.RetryBaseDelaySecs) * time.Second
	}
	if policy.RetryMaxDelaySecs != nil && *policy.RetryMaxDelaySecs > 0 {
		maxDelay = time.Duration(*policy.RetryMaxDelaySecs) * time.Second
	}
	if maxDelay < baseDelay {
		maxDelay = baseDelay
	}

	return maxAttempts, baseDelay, maxDelay
}

func (d *NotifyDispatcher) effectiveEscalationMinInterval(ctx context.Context, state domain.EscalationState) time.Duration {
	interval := d.escalationMinInterval

	policy, err := d.resolvePolicyOverride(ctx, state.ProjectID, state.StepRunID, "", "")
	if err != nil {
		d.logger.Warn("notify dispatcher: resolve escalation policy override", "project_id", state.ProjectID, "escalation_id", state.ID, "error", err)
		return interval
	}
	if policy != nil && policy.EscalationMinIntervalSecs != nil && *policy.EscalationMinIntervalSecs > 0 {
		interval = time.Duration(*policy.EscalationMinIntervalSecs) * time.Second
	}

	return interval
}

func (d *NotifyDispatcher) effectiveDeliveryMaxAttempts(ctx context.Context, projectID, stepRunID, channel string, fallback int) int {
	if fallback <= 0 {
		fallback = 3
	}

	policy, err := d.resolvePolicyOverride(ctx, projectID, stepRunID, "", channel)
	if err != nil {
		d.logger.Warn("notify dispatcher: resolve delivery policy override", "project_id", projectID, "step_run_id", stepRunID, "channel", channel, "error", err)
		return fallback
	}
	if policy != nil && policy.RetryMaxAttempts != nil && *policy.RetryMaxAttempts > 0 {
		return *policy.RetryMaxAttempts
	}
	return fallback
}

func (d *NotifyDispatcher) retryMessage(ctx context.Context, msg domain.NotificationMessage, reason error) bool {
	if !isRetryableDeliveryError(reason) {
		return false
	}

	maxAttempts, baseDelay, maxDelay := d.effectiveRetryPolicy(ctx, msg)
	if msg.Attempts >= maxAttempts {
		return false
	}

	delay := d.computeRetryDelay(msg, baseDelay, maxDelay)
	nextAt := time.Now().UTC().Add(delay)
	err := d.store.UpdateNotificationMessageStatus(ctx, msg.ID, msg.ProjectID, domain.NotifyMessageStatusProcessing, domain.NotifyMessageStatusScheduled, map[string]any{
		"scheduled_at":       nextAt,
		"suppression_reason": reason.Error(),
	})
	if err != nil {
		d.logger.Warn("notify dispatcher: schedule retry failed", "message_id", msg.ID, "error", err)
		return false
	}

	d.logger.Info("notify dispatcher: scheduled retry", "message_id", msg.ID, "attempt", msg.Attempts, "next_at", nextAt)
	if msg.BatchID != "" {
		d.recordDigestRequeue(ctx, "message_retry")
	}
	return true
}

func isRetryableDeliveryError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	nonRetryableFragments := []string{
		"unsupported channel",
		"unsupported email provider",
		"email subject is required",
		"email body is required",
		"subscriber email is required",
		"decode rendered content",
	}
	for _, fragment := range nonRetryableFragments {
		if strings.Contains(message, fragment) {
			return false
		}
	}
	return true
}

func (d *NotifyDispatcher) computeRetryDelay(msg domain.NotificationMessage, baseDelay, maxDelay time.Duration) time.Duration {
	if baseDelay <= 0 {
		baseDelay = d.retryBaseDelay
	}
	if maxDelay <= 0 {
		maxDelay = d.retryMaxDelay
	}
	if maxDelay < baseDelay {
		maxDelay = baseDelay
	}

	attempt := max(msg.Attempts, 1)
	exponent := minInt(attempt-1, 8)
	base := float64(baseDelay)
	delay := time.Duration(base * math.Pow(2, float64(exponent)))
	delay = max(delay, baseDelay)
	delay = min(delay, maxDelay)

	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s:%d", msg.ID, msg.Attempts)
	jitterRatio := float64(h.Sum32()%3000) / 10000.0 // 0.0 - 0.2999
	jittered := time.Duration(float64(delay) * (1 + jitterRatio))
	if jittered > maxDelay {
		return maxDelay
	}
	return jittered
}

func (d *NotifyDispatcher) failMessage(ctx context.Context, msg domain.NotificationMessage, reason error) {
	if reason == nil {
		return
	}
	if msg.BatchID != "" {
		d.recordDigestFailure(ctx, "message_failed")
	}
	if err := d.store.UpdateNotificationMessageStatus(ctx, msg.ID, msg.ProjectID, domain.NotifyMessageStatusProcessing, domain.NotifyMessageStatusFailed, map[string]any{
		"suppression_reason": reason.Error(),
	}); err != nil {
		d.logger.Warn("notify dispatcher: mark failed", "message_id", msg.ID, "error", err)
	}
}
