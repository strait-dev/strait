package api

import (
	"context"
	"reflect"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleBulkCancelAllMapsRequestToStoreFilter(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "project-1")
	req := BulkCancelAllRequest{
		JobID:       "job-1",
		BatchID:     "batch-1",
		TriggeredBy: domain.TriggerManual,
		Status:      domain.StatusQueued,
	}
	wantFilter := store.BulkCancelFilter{
		JobID:       req.JobID,
		BatchID:     req.BatchID,
		TriggeredBy: req.TriggeredBy,
		Status:      req.Status,
	}
	ms := &APIStoreMock{
		BulkCancelByFilterFunc: func(_ context.Context, projectID string, got store.BulkCancelFilter, now time.Time, reason string) ([]string, error) {
			if projectID != "project-1" {
				t.Fatalf("projectID = %q, want project-1", projectID)
			}
			if !reflect.DeepEqual(got, wantFilter) {
				t.Fatalf("filter = %#v, want %#v", got, wantFilter)
			}
			if now.IsZero() {
				t.Fatal("now must be set")
			}
			if reason != "canceled by user (bulk filter)" {
				t.Fatalf("reason = %q, want bulk filter cancel reason", reason)
			}
			return []string{"run-1", "run-2"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	output, err := srv.handleBulkCancelAll(ctx, &BulkCancelAllInput{Body: req})
	if err != nil {
		t.Fatalf("handleBulkCancelAll() error = %v", err)
	}

	if got := output.Body["canceled"]; got != 2 {
		t.Fatalf("canceled = %#v, want 2", got)
	}
	if got, ok := output.Body["run_ids"].([]string); !ok || !reflect.DeepEqual(got, []string{"run-1", "run-2"}) {
		t.Fatalf("run_ids = %#v, want run-1/run-2", output.Body["run_ids"])
	}
}

func TestHandleBulkCancelAllSkipsStoreWhenFilterMissing(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		BulkCancelByFilterFunc: func(context.Context, string, store.BulkCancelFilter, time.Time, string) ([]string, error) {
			t.Fatal("BulkCancelByFilter must not be called without a filter")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	_, err := srv.handleBulkCancelAll(context.Background(), &BulkCancelAllInput{Body: BulkCancelAllRequest{}})
	if !isHumaStatusError(err, 400) {
		t.Fatalf("expected 400 for missing filters, got %v", err)
	}
}
