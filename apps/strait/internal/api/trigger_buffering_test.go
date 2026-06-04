package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
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

	if pending.JobID != job.ID || pending.ProjectID != job.ProjectID {
		t.Fatalf("job/project = (%q, %q), want (%q, %q)", pending.JobID, pending.ProjectID, job.ID, job.ProjectID)
	}
	if pending.DebounceKey != req.DebounceKey {
		t.Fatalf("debounce key = %q, want %q", pending.DebounceKey, req.DebounceKey)
	}
	if string(pending.Payload) != `{"customer_id":"customer-1"}` {
		t.Fatalf("payload = %s", pending.Payload)
	}
	if !jsonEqual(pending.Tags, json.RawMessage(`{"team":"ops","tier":"gold"}`)) {
		t.Fatalf("tags = %s", pending.Tags)
	}
	if pending.Priority != req.Priority {
		t.Fatalf("priority = %d, want %d", pending.Priority, req.Priority)
	}
	if pending.ConcurrencyKey != req.ConcurrencyKey {
		t.Fatalf("concurrency key = %q, want %q", pending.ConcurrencyKey, req.ConcurrencyKey)
	}
	if pending.TTLSecs == nil || *pending.TTLSecs != ttlSecs {
		t.Fatalf("ttl_secs = %v, want %d", pending.TTLSecs, ttlSecs)
	}
	if pending.TriggeredBy != domain.TriggerDebounce {
		t.Fatalf("triggered_by = %q, want debounce", pending.TriggeredBy)
	}
	if pending.CreatedBy != "user-1" {
		t.Fatalf("created_by = %q, want user-1", pending.CreatedBy)
	}
	if !pending.FireAt.Equal(now.Add(45 * time.Second)) {
		t.Fatalf("fire_at = %v, want %v", pending.FireAt, now.Add(45*time.Second))
	}
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

	if item.JobID != job.ID || item.ProjectID != job.ProjectID {
		t.Fatalf("job/project = (%q, %q), want (%q, %q)", item.JobID, item.ProjectID, job.ID, job.ProjectID)
	}
	if item.BatchKey != req.BatchKey {
		t.Fatalf("batch key = %q, want %q", item.BatchKey, req.BatchKey)
	}
	if string(item.Payload) != `{"n":1}` {
		t.Fatalf("payload = %s", item.Payload)
	}
	if !jsonEqual(item.Tags, json.RawMessage(`{"batch":"daily"}`)) {
		t.Fatalf("tags = %s", item.Tags)
	}
	if item.Priority != req.Priority {
		t.Fatalf("priority = %d, want %d", item.Priority, req.Priority)
	}
	if item.TriggeredBy != domain.TriggerManual {
		t.Fatalf("triggered_by = %q, want manual", item.TriggeredBy)
	}
	if item.CreatedBy != "apikey:batch" {
		t.Fatalf("created_by = %q, want apikey:batch", item.CreatedBy)
	}
}

func TestHandleDebounceTriggerSkipsWhenDisabled(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		UpsertDebouncePendingFunc: func(context.Context, *domain.DebouncePending) error {
			t.Fatal("UpsertDebouncePending must not run when debounce is disabled")
			return nil
		},
	}}
	out, handled, err := srv.handleDebounceTrigger(context.Background(), &triggerRequestState{
		job: &domain.Job{ID: "job-1", ProjectID: "project-1"},
	})
	if err != nil {
		t.Fatalf("handleDebounceTrigger() error = %v", err)
	}
	if handled || out != nil {
		t.Fatalf("handleDebounceTrigger() = (%v, %v), want nil output and handled=false", out, handled)
	}
}

func TestHandleBatchTriggerBuffersWhenWindowEnabled(t *testing.T) {
	t.Parallel()

	inserted := false
	srv := &Server{store: &APIStoreMock{
		InsertBatchBufferItemFunc: func(_ context.Context, item *domain.BatchBufferItem) error {
			inserted = true
			if item.JobID != "job-batch" || item.BatchKey != "customer-1" {
				t.Fatalf("buffer item = %+v, want job-batch/customer-1", item)
			}
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
	if err != nil {
		t.Fatalf("handleBatchTrigger() error = %v", err)
	}
	if !handled {
		t.Fatal("handleBatchTrigger() handled = false, want true")
	}
	if !inserted {
		t.Fatal("batch buffer item was not inserted")
	}
	body, ok := out.Body.(map[string]any)
	if !ok {
		t.Fatalf("output body = %T, want map[string]any", out.Body)
	}
	if body["buffered"] != true {
		t.Fatalf("buffered = %v, want true", body["buffered"])
	}
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
