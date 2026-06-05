package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestNewDebouncePending_BuildsBufferedTrigger(t *testing.T) {
	t.Parallel()

	ttlSecs := 120
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	ctx := context.WithValue(context.Background(), ctxActorIDKey, "user-1")
	job := &domain.Job{
		ID:                 "job-debounce",
		ProjectID:          "project-1",
		DebounceWindowSecs: 45,
	}
	req := TriggerRequest{
		Tags:           map[string]string{"team": "ops", "tier": "gold"},
		Priority:       9,
		TTLSecs:        &ttlSecs,
		DebounceKey:    "customer-1",
		ConcurrencyKey: "customer-1",
	}

	pending := newDebouncePending(ctx, debouncePendingRequest{
		job:     job,
		req:     req,
		payload: json.RawMessage(`{"customer_id":"customer-1"}`),
		now:     now,
	})
	require.False(t, pending.JobID !=
		job.ID ||
		pending.
			ProjectID != job.ProjectID,
	)
	require.Equal(t, req.DebounceKey,
		pending.
			DebounceKey,
	)
	require.Equal(t, `{"customer_id":"customer-1"}`,

		string(pending.Payload))
	require.True(
		t, jsonEqual(pending.
			Tags, json.
			RawMessage(`{"team":"ops","tier":"gold"}`)))
	require.Equal(t, req.Priority,
		pending.Priority,
	)
	require.Equal(t, req.ConcurrencyKey,
		pending.
			ConcurrencyKey,
	)
	require.False(t, pending.TTLSecs ==
		nil ||
		*pending.
			TTLSecs != ttlSecs)
	require.Equal(t, domain.TriggerDebounce,

		pending.
			TriggeredBy,
	)
	require.Equal(t, "user-1", pending.
		CreatedBy,
	)
	require.True(
		t, pending.FireAt.
			Equal(now.
				Add(45*

					time.Second)))

}

func TestNewBatchBufferItem_BuildsBufferedTrigger(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxActorIDKey, "apikey:batch")
	job := &domain.Job{
		ID:        "job-batch",
		ProjectID: "project-1",
	}
	req := TriggerRequest{
		Tags:     map[string]string{"batch": "daily"},
		Priority: 4,
		BatchKey: "customer-1",
	}

	item := newBatchBufferItem(ctx, batchBufferItemRequest{
		job:     job,
		req:     req,
		payload: json.RawMessage(`{"n":1}`),
	})
	require.False(t, item.JobID !=
		job.ID ||
		item.ProjectID !=
			job.ProjectID)
	require.Equal(t, req.BatchKey,
		item.BatchKey,
	)
	require.Equal(t, `{"n":1}`, string(item.Payload))
	require.True(
		t, jsonEqual(item.
			Tags, json.
			RawMessage(`{"batch":"daily"}`)))
	require.Equal(t, req.Priority,
		item.Priority,
	)
	require.Equal(t, domain.TriggerManual,
		item.
			TriggeredBy,
	)
	require.Equal(t, "apikey:batch",
		item.CreatedBy,
	)

}

func TestHandleDebounceTriggerSkipsWhenDisabled(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		UpsertDebouncePendingFunc: func(context.Context, *domain.DebouncePending) error {
			require.Fail(t,

				"UpsertDebouncePending must not run when debounce is disabled")
			return nil
		},
	}}
	out, handled, err := srv.handleDebounceTrigger(context.Background(), &triggerRequestState{
		job: &domain.Job{ID: "job-1", ProjectID: "project-1"},
	})
	require.NoError(t, err)
	require.False(t, handled || out !=
		nil)

}

func TestHandleBatchTriggerBuffersWhenWindowEnabled(t *testing.T) {
	t.Parallel()

	inserted := false
	srv := &Server{store: &APIStoreMock{
		InsertBatchBufferItemFunc: func(_ context.Context, item *domain.BatchBufferItem) error {
			inserted = true
			require.False(t, item.JobID !=
				"job-batch" ||
				item.
					BatchKey != "customer-1")

			return nil
		},
	}}
	ctx := context.WithValue(context.Background(), ctxActorIDKey, "apikey:batch")
	out, handled, err := srv.handleBatchTrigger(ctx, &TriggerJobInput{}, &triggerRequestState{
		job: &domain.Job{
			ID:              "job-batch",
			ProjectID:       "project-1",
			BatchWindowSecs: 60,
		},
		req: TriggerRequest{
			BatchKey: "customer-1",
			Priority: 3,
			Tags:     map[string]string{"kind": "daily"},
		},
		payload: json.RawMessage(`{"n":1}`),
	})
	require.NoError(t, err)
	require.True(
		t, handled)
	require.True(
		t, inserted)

	body, ok := out.Body.(map[string]any)
	require.True(
		t, ok)
	require.Equal(t, true, body["buffered"])

}

func jsonEqual(left, right json.RawMessage) bool {
	var leftValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	var rightValue any
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	return jsonString(leftValue) == jsonString(rightValue)
}

func jsonString(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
