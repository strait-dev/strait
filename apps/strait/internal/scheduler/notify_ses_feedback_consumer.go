package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awsv2config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

const notifySESFeedbackReceiptTTL = 30 * 24 * time.Hour

type notifySESFeedbackStore interface {
	GetNotificationMessageByID(ctx context.Context, id string) (*domain.NotificationMessage, error)
	UpdateNotificationMessageStatus(ctx context.Context, id, projectID, fromStatus, toStatus string, fields map[string]any) error
	DisableNotificationChannelPreference(ctx context.Context, recipientType, recipientID, scope, channel string) error
	CreateNotifySuppressionEvent(ctx context.Context, event *domain.NotifySuppressionEvent) error
	RecordNotifyProviderCallbackReceipt(ctx context.Context, projectID, providerID, provider, callbackID, eventType, messageID, payloadHash string, expiresAt time.Time) (bool, error)
	DeleteNotifyProviderCallbackReceipt(ctx context.Context, projectID, providerID, callbackID string) error
}

type notifySESFeedbackSQSAPI interface {
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

type NotifySESFeedbackConsumerConfig struct {
	QueueURL                 string
	Region                   string
	PollInterval             time.Duration
	WaitTimeSeconds          int32
	MaxMessages              int32
	VisibilityTimeoutSeconds int32
	AccessKeyID              string
	SecretAccessKey          string
	SessionToken             string
}

type NotifySESFeedbackConsumer struct {
	store  notifySESFeedbackStore
	client notifySESFeedbackSQSAPI
	cfg    NotifySESFeedbackConsumerConfig
	logger *slog.Logger
}

func NewNotifySESFeedbackConsumer(store notifySESFeedbackStore, cfg NotifySESFeedbackConsumerConfig) (*NotifySESFeedbackConsumer, error) {
	loadOptions := []func(*awsv2config.LoadOptions) error{}
	if strings.TrimSpace(cfg.Region) != "" {
		loadOptions = append(loadOptions, awsv2config.WithRegion(strings.TrimSpace(cfg.Region)))
	}
	if strings.TrimSpace(cfg.AccessKeyID) != "" || strings.TrimSpace(cfg.SecretAccessKey) != "" {
		loadOptions = append(loadOptions, awsv2config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			cfg.SessionToken,
		)))
	}

	awsCfg, err := awsv2config.LoadDefaultConfig(context.Background(), loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config for notify ses feedback consumer: %w", err)
	}

	return NewNotifySESFeedbackConsumerWithClient(store, sqs.NewFromConfig(awsCfg), cfg), nil
}

func NewNotifySESFeedbackConsumerWithClient(store notifySESFeedbackStore, client notifySESFeedbackSQSAPI, cfg NotifySESFeedbackConsumerConfig) *NotifySESFeedbackConsumer {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.WaitTimeSeconds <= 0 {
		cfg.WaitTimeSeconds = 10
	}
	if cfg.MaxMessages <= 0 || cfg.MaxMessages > 10 {
		cfg.MaxMessages = 10
	}
	if cfg.VisibilityTimeoutSeconds <= 0 {
		cfg.VisibilityTimeoutSeconds = 120
	}

	return &NotifySESFeedbackConsumer{
		store:  store,
		client: client,
		cfg:    cfg,
		logger: slog.Default(),
	}
}

func (c *NotifySESFeedbackConsumer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		processed, err := c.pollOnce(ctx)
		if err != nil {
			c.logger.Warn("notify ses feedback consumer: poll failed", "error", err)
		}
		if processed > 0 {
			continue
		}

		timer := time.NewTimer(c.cfg.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (c *NotifySESFeedbackConsumer) pollOnce(ctx context.Context) (int, error) {
	resp, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            awsv2.String(c.cfg.QueueURL),
		MaxNumberOfMessages: c.cfg.MaxMessages,
		WaitTimeSeconds:     c.cfg.WaitTimeSeconds,
		VisibilityTimeout:   c.cfg.VisibilityTimeoutSeconds,
	})
	if err != nil {
		return 0, fmt.Errorf("receive sqs message: %w", err)
	}
	if resp == nil || len(resp.Messages) == 0 {
		return 0, nil
	}

	processed := 0
	for _, message := range resp.Messages {
		shouldDelete, processErr := c.processMessage(ctx, message)
		if processErr != nil {
			c.logger.Warn("notify ses feedback consumer: process message failed",
				"message_id", awsv2.ToString(message.MessageId),
				"error", processErr,
			)
			continue
		}
		if !shouldDelete {
			continue
		}
		if _, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      awsv2.String(c.cfg.QueueURL),
			ReceiptHandle: message.ReceiptHandle,
		}); err != nil {
			c.logger.Warn("notify ses feedback consumer: delete message failed",
				"message_id", awsv2.ToString(message.MessageId),
				"error", err,
			)
			continue
		}
		processed++
	}

	return processed, nil
}

func (c *NotifySESFeedbackConsumer) processMessage(ctx context.Context, sqsMessage sqstypes.Message) (bool, error) {
	rawBody := []byte(awsv2.ToString(sqsMessage.Body))
	if len(rawBody) == 0 {
		return true, nil
	}

	event, callbackID, eventType, parseErr := parseNotifySESEvent(rawBody)
	if parseErr != nil {
		c.logger.Warn("notify ses feedback consumer: malformed event", "error", parseErr)
		return true, nil
	}
	if callbackID == "" {
		callbackID = awsv2.ToString(sqsMessage.MessageId)
	}
	if callbackID == "" {
		callbackID = hashNotifySESFeedbackPayload(rawBody)
	}

	straitMessageID := event.straitMessageID()
	if straitMessageID == "" {
		return true, nil
	}

	msg, err := c.store.GetNotificationMessageByID(ctx, straitMessageID)
	if err != nil {
		if errors.Is(err, store.ErrNotificationMessageNotFound) {
			return true, nil
		}
		return false, fmt.Errorf("resolve notification message: %w", err)
	}
	if msg == nil {
		return true, nil
	}
	if taggedProjectID := strings.TrimSpace(event.mailTag("strait_project_id")); taggedProjectID != "" && taggedProjectID != msg.ProjectID {
		return true, nil
	}

	providerID := strings.TrimSpace(msg.ProviderID)
	if providerID == "" {
		providerID = "ses_default"
	}

	recorded, err := c.store.RecordNotifyProviderCallbackReceipt(
		ctx,
		msg.ProjectID,
		providerID,
		"ses",
		callbackID,
		eventType,
		msg.ID,
		hashNotifySESFeedbackPayload(rawBody),
		time.Now().UTC().Add(notifySESFeedbackReceiptTTL),
	)
	if err != nil {
		return false, fmt.Errorf("record notify provider callback receipt: %w", err)
	}
	if !recorded {
		return true, nil
	}

	processed := false
	defer func() {
		if processed {
			return
		}
		if cleanupErr := c.store.DeleteNotifyProviderCallbackReceipt(ctx, msg.ProjectID, providerID, callbackID); cleanupErr != nil {
			c.logger.Warn("notify ses feedback consumer: callback receipt cleanup failed",
				"project_id", msg.ProjectID,
				"provider_id", providerID,
				"callback_id", callbackID,
				"error", cleanupErr,
			)
		}
	}()

	nextStatus, fields, suppressEmail, suppressionReason := resolveNotifySESFeedbackOutcome(eventType, event.eventTimestamp())
	if nextStatus == "" {
		processed = true
		return true, nil
	}

	if !shouldApplyNotifySESTransition(msg.Status, nextStatus) {
		c.applyNotifySESSuppressionIfNeeded(ctx, msg, suppressEmail, suppressionReason, callbackID, eventType)
		processed = true
		return true, nil
	}

	if err := c.store.UpdateNotificationMessageStatus(ctx, msg.ID, msg.ProjectID, msg.Status, nextStatus, fields); err != nil {
		if errors.Is(err, store.ErrNotificationMessageStatusConflict) {
			latest, latestErr := c.store.GetNotificationMessageByID(ctx, msg.ID)
			if latestErr == nil && latest != nil && !shouldApplyNotifySESTransition(latest.Status, nextStatus) {
				c.applyNotifySESSuppressionIfNeeded(ctx, latest, suppressEmail, suppressionReason, callbackID, eventType)
				processed = true
				return true, nil
			}
		}
		return false, fmt.Errorf("update notification message status: %w", err)
	}

	c.applyNotifySESSuppressionIfNeeded(ctx, msg, suppressEmail, suppressionReason, callbackID, eventType)
	processed = true
	return true, nil
}

func (c *NotifySESFeedbackConsumer) applyNotifySESSuppressionIfNeeded(
	ctx context.Context,
	msg *domain.NotificationMessage,
	suppressEmail bool,
	reason,
	callbackID,
	eventType string,
) {
	if !suppressEmail || msg == nil {
		return
	}
	if msg.RecipientType != domain.NotifyRecipientTypeSubscriber || msg.RecipientID == "" {
		return
	}

	if err := c.store.DisableNotificationChannelPreference(ctx, domain.NotifyRecipientTypeSubscriber, msg.RecipientID, "global", "email"); err != nil {
		c.logger.Warn("notify ses feedback consumer: disable email preference failed", "message_id", msg.ID, "error", err)
		return
	}

	metadata, err := json.Marshal(map[string]any{
		"callback_id": callbackID,
		"event_type":  eventType,
		"message_id":  msg.ID,
	})
	if err != nil {
		metadata = []byte("{}")
	}

	event := &domain.NotifySuppressionEvent{
		ProjectID:     msg.ProjectID,
		RecipientType: domain.NotifyRecipientTypeSubscriber,
		RecipientID:   msg.RecipientID,
		Scope:         "global",
		Channel:       "email",
		Action:        domain.NotifySuppressionActionSuppressed,
		Reason:        reason,
		Source:        domain.NotifySuppressionSourceProviderCallback,
		Metadata:      metadata,
	}
	if err := c.store.CreateNotifySuppressionEvent(ctx, event); err != nil {
		c.logger.Warn("notify ses feedback consumer: create suppression event failed", "message_id", msg.ID, "error", err)
	}
}

func shouldApplyNotifySESTransition(currentStatus, nextStatus string) bool {
	if strings.TrimSpace(nextStatus) == "" {
		return false
	}
	if strings.EqualFold(currentStatus, nextStatus) {
		return false
	}
	if isNotifySESTerminalStatus(currentStatus) {
		return false
	}
	return true
}

func isNotifySESTerminalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case domain.NotifyMessageStatusDelivered,
		domain.NotifyMessageStatusFailed,
		domain.NotifyMessageStatusBounced,
		domain.NotifyMessageStatusCancelled:
		return true
	default:
		return false
	}
}

func resolveNotifySESFeedbackOutcome(eventType string, ts time.Time) (string, map[string]any, bool, string) {
	normalized := strings.ToLower(strings.TrimSpace(eventType))
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	switch normalized {
	case "bounce":
		reason := "provider_callback:ses.bounce"
		return domain.NotifyMessageStatusBounced, map[string]any{
			"bounced_at":         ts,
			"suppression_reason": reason,
		}, true, reason
	case "complaint":
		reason := "provider_callback:ses.complaint"
		return domain.NotifyMessageStatusFailed, map[string]any{
			"suppression_reason": reason,
		}, true, reason
	case "delivery":
		return domain.NotifyMessageStatusDelivered, map[string]any{
			"delivered_at": ts,
		}, false, ""
	default:
		return "", nil, false, ""
	}
}

func hashNotifySESFeedbackPayload(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

type notifySNSMessageEnvelope struct {
	Type      string `json:"Type"`
	MessageID string `json:"MessageId"`
	Message   string `json:"Message"`
}

type notifySESEvent struct {
	EventType string `json:"eventType"`
	Mail      struct {
		MessageID string              `json:"messageId"`
		Timestamp string              `json:"timestamp"`
		Tags      map[string][]string `json:"tags"`
	} `json:"mail"`
	Bounce struct {
		Timestamp string `json:"timestamp"`
	} `json:"bounce"`
	Complaint struct {
		Timestamp string `json:"timestamp"`
	} `json:"complaint"`
	Delivery struct {
		Timestamp string `json:"timestamp"`
	} `json:"delivery"`
}

func parseNotifySESEvent(rawBody []byte) (*notifySESEvent, string, string, error) {
	body := rawBody
	callbackID := ""

	envelope := notifySNSMessageEnvelope{}
	if err := json.Unmarshal(rawBody, &envelope); err == nil {
		if strings.EqualFold(strings.TrimSpace(envelope.Type), "notification") && strings.TrimSpace(envelope.Message) != "" {
			body = []byte(envelope.Message)
			callbackID = strings.TrimSpace(envelope.MessageID)
		}
	}

	event := &notifySESEvent{}
	if err := json.Unmarshal(body, event); err != nil {
		return nil, callbackID, "", fmt.Errorf("unmarshal ses event: %w", err)
	}

	eventType := strings.ToLower(strings.TrimSpace(event.EventType))
	if eventType == "" {
		return nil, callbackID, "", fmt.Errorf("missing ses eventType")
	}

	return event, callbackID, eventType, nil
}

func (e *notifySESEvent) straitMessageID() string {
	if e == nil {
		return ""
	}
	if v := e.mailTag("strait_message_id"); v != "" {
		return v
	}
	return strings.TrimSpace(e.Mail.MessageID)
}

func (e *notifySESEvent) mailTag(key string) string {
	if e == nil {
		return ""
	}
	values := e.Mail.Tags[key]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func (e *notifySESEvent) eventTimestamp() time.Time {
	if e == nil {
		return time.Time{}
	}

	candidates := []string{
		e.Bounce.Timestamp,
		e.Complaint.Timestamp,
		e.Delivery.Timestamp,
		e.Mail.Timestamp,
	}
	for _, raw := range candidates {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			return ts.UTC()
		}
		if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return ts.UTC()
		}
	}

	return time.Now().UTC()
}
