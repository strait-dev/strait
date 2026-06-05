package billing

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDispatcher struct {
	mu         sync.Mutex
	calls      []fakeDispatchCall
	err        error
	onDispatch func()
}

type fakeDispatchCall struct {
	orgID     string
	eventType string
	payload   []byte
}

func (f *fakeDispatcher) DispatchBillingEvent(_ context.Context, orgID, eventType string, payload []byte) error {
	f.mu.Lock()
	f.calls = append(f.calls, fakeDispatchCall{orgID: orgID, eventType: eventType, payload: payload})
	err := f.err
	onDispatch := f.onDispatch
	f.mu.Unlock()
	if onDispatch != nil {
		onDispatch()
	}
	return err
}

func TestDispatchBillingWebhook_PayloadShape(t *testing.T) {
	t.Parallel()

	d := &fakeDispatcher{}
	err := DispatchBillingWebhook(context.Background(), d,
		"org_123", domain.PlanScale, domain.WebhookEventBillingCapWarning,
		map[string]any{"spend_pct": 0.81, "limit_microusd": int64(500_000_000)},
	)
	require.NoError(t,
		err)
	require.Len(t, d.
		calls, 1)

	call := d.calls[0]
	assert.Equal(t, "org_123",
		call.orgID,
	)
	assert.Equal(t, domain.
		WebhookEventBillingCapWarning,

		call.
			eventType,
	)

	var env BillingEventEnvelope
	require.NoError(t,
		json.Unmarshal(
			call.payload,
			&env))
	assert.NotEmpty(t,
		env.EventID,
	)
	assert.Equal(t, domain.
		WebhookEventBillingCapWarning,

		env.
			EventType,
	)
	assert.Equal(t, "org_123",
		env.OrgID,
	)
	assert.Equal(t, string(domain.PlanScale), env.
		PlanTier)
	assert.NotEmpty(t,
		env.OccurredAt,
	)
	assert.NotNil(t,
		env.Detail["spend_pct"])
}

func TestDispatchBillingWebhook_PropagatesDispatcherError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("delivery store down")
	d := &fakeDispatcher{err: sentinel}
	err := DispatchBillingWebhook(context.Background(), d,
		"org", domain.PlanPro, domain.WebhookEventBillingDelinquent, nil)
	assert.ErrorIs(t, err, sentinel)
}

func TestDispatchBillingWebhook_ValidatesInputs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	require.Error(t, DispatchBillingWebhook(ctx,
		nil, "o", domain.
			PlanFree,
		"x",

		nil))

	d := &fakeDispatcher{}
	require.Error(t, DispatchBillingWebhook(ctx,
		d, "", domain.
			PlanFree,
		"x", nil,
	))
	assert.Error(t, DispatchBillingWebhook(ctx,
		d, "o", domain.
			PlanFree,
		"", nil,
	))
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
		assert.Equal(t, w,
			got[name])
		assert.Contains(t, w, ".")
	}
}

func TestEnforcer_WithBillingDispatcher(t *testing.T) {
	t.Parallel()

	d := &fakeDispatcher{}
	e := &Enforcer{}
	WithBillingDispatcher(d)(e)
	require.NotNil(t,
		e.billingDispatcher,
	)
}
