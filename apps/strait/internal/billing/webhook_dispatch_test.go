package billing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"strait/internal/domain"
)

type fakeDispatcher struct {
	calls []fakeDispatchCall
	err   error
}

type fakeDispatchCall struct {
	orgID     string
	eventType string
	payload   []byte
}

func (f *fakeDispatcher) DispatchBillingEvent(_ context.Context, orgID, eventType string, payload []byte) error {
	f.calls = append(f.calls, fakeDispatchCall{orgID: orgID, eventType: eventType, payload: payload})
	return f.err
}

func TestDispatchBillingWebhook_PayloadShape(t *testing.T) {
	t.Parallel()

	d := &fakeDispatcher{}
	err := DispatchBillingWebhook(context.Background(), d,
		"org_123", domain.PlanScale, domain.WebhookEventBillingCapWarning,
		map[string]any{"spend_pct": 0.81, "limit_microusd": int64(500_000_000)},
	)
	if err != nil {
		t.Fatalf("DispatchBillingWebhook err = %v", err)
	}
	if len(d.calls) != 1 {
		t.Fatalf("dispatcher calls = %d, want 1", len(d.calls))
	}
	call := d.calls[0]
	if call.orgID != "org_123" {
		t.Errorf("orgID = %q, want org_123", call.orgID)
	}
	if call.eventType != domain.WebhookEventBillingCapWarning {
		t.Errorf("eventType = %q, want %q", call.eventType, domain.WebhookEventBillingCapWarning)
	}
	var env BillingEventEnvelope
	if err := json.Unmarshal(call.payload, &env); err != nil {
		t.Fatalf("payload not valid JSON: %v (%q)", err, call.payload)
	}
	if env.EventID == "" {
		t.Error("event_id missing")
	}
	if env.EventType != domain.WebhookEventBillingCapWarning {
		t.Errorf("env.event_type = %q", env.EventType)
	}
	if env.OrgID != "org_123" {
		t.Errorf("env.org_id = %q", env.OrgID)
	}
	if env.PlanTier != string(domain.PlanScale) {
		t.Errorf("env.plan_tier = %q, want %q", env.PlanTier, domain.PlanScale)
	}
	if env.OccurredAt == "" {
		t.Error("occurred_at missing")
	}
	if env.Detail["spend_pct"] == nil {
		t.Errorf("detail.spend_pct missing: %+v", env.Detail)
	}
}

func TestDispatchBillingWebhook_PropagatesDispatcherError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("delivery store down")
	d := &fakeDispatcher{err: sentinel}
	err := DispatchBillingWebhook(context.Background(), d,
		"org", domain.PlanPro, domain.WebhookEventBillingDelinquent, nil)
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want sentinel error", err)
	}
}

func TestDispatchBillingWebhook_ValidatesInputs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if err := DispatchBillingWebhook(ctx, nil, "o", domain.PlanFree, "x", nil); err == nil {
		t.Error("nil dispatcher should error")
	}
	d := &fakeDispatcher{}
	if err := DispatchBillingWebhook(ctx, d, "", domain.PlanFree, "x", nil); err == nil {
		t.Error("empty orgID should error")
	}
	if err := DispatchBillingWebhook(ctx, d, "o", domain.PlanFree, "", nil); err == nil {
		t.Error("empty eventType should error")
	}
}

func TestValidWebhookEventTypes_IncludesBilling(t *testing.T) {
	t.Parallel()

	// The set is in the api package, but we assert via the constants in
	// domain that every new constant has a stable string value (no rename
	// without a coordinated schema bump).
	want := map[string]string{
		"WebhookEventBillingCapWarning":            "billing.cap_warning",
		"WebhookEventBillingCapReached":            "billing.cap_reached",
		"WebhookEventBillingCapDisabled":           "billing.cap_disabled",
		"WebhookEventBillingOverageDisabled":       "billing.overage_disabled",
		"WebhookEventBillingSuspended":             "billing.suspended",
		"WebhookEventBillingDelinquent":            "billing.delinquent",
		"WebhookEventScheduleSuspended":            "schedule.suspended",
		"WebhookEventWorkflowRegistrationRejected": "workflow.registration_rejected",
		"WebhookEventSLACreditIssued":              "sla.credit_issued",
	}
	got := map[string]string{
		"WebhookEventBillingCapWarning":            domain.WebhookEventBillingCapWarning,
		"WebhookEventBillingCapReached":            domain.WebhookEventBillingCapReached,
		"WebhookEventBillingCapDisabled":           domain.WebhookEventBillingCapDisabled,
		"WebhookEventBillingOverageDisabled":       domain.WebhookEventBillingOverageDisabled,
		"WebhookEventBillingSuspended":             domain.WebhookEventBillingSuspended,
		"WebhookEventBillingDelinquent":            domain.WebhookEventBillingDelinquent,
		"WebhookEventScheduleSuspended":            domain.WebhookEventScheduleSuspended,
		"WebhookEventWorkflowRegistrationRejected": domain.WebhookEventWorkflowRegistrationRejected,
		"WebhookEventSLACreditIssued":              domain.WebhookEventSLACreditIssued,
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %q, want %q (string is wire-stable; change requires schema bump)",
				name, got[name], w)
		}
		if !strings.Contains(w, ".") {
			t.Errorf("event type %q must contain a dotted namespace", w)
		}
	}
}

func TestEnforcer_WithBillingDispatcher(t *testing.T) {
	t.Parallel()

	d := &fakeDispatcher{}
	e := &Enforcer{}
	WithBillingDispatcher(d)(e)
	if e.billingDispatcher == nil {
		t.Fatal("billingDispatcher not set")
	}
}
