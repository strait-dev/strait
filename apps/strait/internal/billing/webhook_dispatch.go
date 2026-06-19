package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"strait/internal/domain"
)

// BillingWebhookMaxPayloadBytes caps the serialized payload size for
// billing webhook events. The cap matches the platform-wide webhook
// payload ceiling and is high enough to hold any structured billing
// detail we emit, but low enough to short-circuit accidental
// runaway-blob detail values before they hit the delivery queue.
var BillingWebhookMaxPayloadBytes = billingWebhookMaxPayloadBytes()

func billingWebhookMaxPayloadBytes() int {
	return 64 * 1024
}

// BillingEventDispatcher delivers a fully-formed billing event payload
// to the outbound webhook pipeline (webhook_subscriptions). The concrete
// implementation lives outside the billing package so this package
// stays free of the webhook store + delivery dependencies.
type BillingEventDispatcher interface {
	DispatchBillingEvent(ctx context.Context, orgID, eventType string, payload []byte) error
}

// BillingEventEnvelope is the wire format every outbound billing event
// shares. Helpers and the dispatch site MUST produce a payload of this
// shape so subscribers can rely on the common header fields.
type BillingEventEnvelope struct {
	EventID    string         `json:"event_id"`
	EventType  string         `json:"event_type"`
	OrgID      string         `json:"org_id"`
	PlanTier   string         `json:"plan_tier"`
	OccurredAt string         `json:"occurred_at"`
	Detail     map[string]any `json:"detail,omitempty"`
}

// DispatchBillingWebhook builds the canonical billing-event envelope,
// rejects payloads that exceed BillingWebhookMaxPayloadBytes, and hands
// the bytes to the dispatcher. It returns the dispatcher's error
// unchanged so callers can fold delivery failures into their own
// retry/log policy.
func DispatchBillingWebhook(
	ctx context.Context,
	d BillingEventDispatcher,
	orgID string,
	planTier domain.PlanTier,
	eventType string,
	detail map[string]any,
) error {
	if d == nil {
		return errors.New("billing: dispatcher is nil")
	}
	if orgID == "" {
		return errors.New("billing: orgID is required")
	}
	if eventType == "" {
		return errors.New("billing: eventType is required")
	}

	env := BillingEventEnvelope{
		EventID:    uuid.NewString(),
		EventType:  eventType,
		OrgID:      orgID,
		PlanTier:   string(planTier),
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		Detail:     detail,
	}
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("billing: marshal envelope: %w", err)
	}
	if len(body) > BillingWebhookMaxPayloadBytes {
		return fmt.Errorf("billing: payload %d bytes exceeds %d-byte cap",
			len(body), BillingWebhookMaxPayloadBytes)
	}
	return d.DispatchBillingEvent(ctx, orgID, eventType, body)
}
