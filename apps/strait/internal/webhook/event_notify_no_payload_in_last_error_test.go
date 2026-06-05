package webhook

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression guard: EnqueueRunWebhook must not stash the JSON payload in
// LastError. LastError is reserved for actual delivery failures; mixing the
// payload in there made retry diagnostics ambiguous and inflated the column
// for every healthy delivery.
func TestEnqueueRunWebhook_DoesNotStashPayloadInLastError(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	job := &domain.Job{
		ID:         "job-1",
		ProjectID:  "proj-1",
		WebhookURL: "http://example.com/hook",
	}
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusCompleted,
		Attempt:   1,
		Result:    json.RawMessage(`{"ok":true}`),
	}
	require.NoError(t,
		worker.EnqueueRunWebhook(context.
			Background(), job, run))

	deliveries := ms.getDeliveries()
	require.Len(t, deliveries,
		1)

	d := deliveries[0]
	assert.Equal(t, "",
		d.LastError)
	assert.NotEmpty(t,
		d.Payload)

}

// Regression guard: EnqueueSubscriptionWebhooks must not stash the payload in
// LastError either. Same reasoning as EnqueueRunWebhook.
func TestEnqueueSubscriptionWebhooks_DoesNotStashPayloadInLastError(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	subs := []domain.WebhookSubscription{
		{
			ID:         "sub-1",
			ProjectID:  "proj-1",
			WebhookURL: "http://example.com/hook",
			EventTypes: []string{"billing.cap_warning"},
			Active:     true,
		},
	}
	payload := json.RawMessage(`{"event":"billing.cap_warning","org_id":"org-1"}`)

	worker.EnqueueSubscriptionWebhooks(context.Background(), subs, "billing.cap_warning", payload)

	deliveries := ms.getDeliveries()
	require.Len(t, deliveries,
		1)

	d := deliveries[0]
	assert.Equal(t, "",
		d.LastError)
	assert.Equal(t, string(payload), string(d.Payload))

}
