package api

import (
	"context"
	"reflect"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
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
			require.Equal(t, "project-1",
				projectID,
			)
			require.True(
				t, reflect.
					DeepEqual(got,
						wantFilter,
					))
			require.False(t, now.IsZero())
			require.Equal(t, "canceled by user (bulk filter)",

				reason)

			return []string{"run-1", "run-2"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	output, err := srv.handleBulkCancelAll(ctx, &BulkCancelAllInput{Body: req})
	require.NoError(t, err)
	require.EqualValues(t, 2, output.
		Body["canceled"],
	)

	if got, ok := output.Body["run_ids"].([]string); !ok || !reflect.DeepEqual(got, []string{"run-1", "run-2"}) {
		require.Failf(t, "test failure",

			"run_ids = %#v, want run-1/run-2", output.Body["run_ids"])
	}
}

func TestHandleBulkCancelAllSkipsStoreWhenFilterMissing(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		BulkCancelByFilterFunc: func(context.Context, string, store.BulkCancelFilter, time.Time, string) ([]string, error) {
			require.Fail(t,

				"BulkCancelByFilter must not be called without a filter")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	_, err := srv.handleBulkCancelAll(context.Background(), &BulkCancelAllInput{Body: BulkCancelAllRequest{}})
	require.True(
		t, isHumaStatusError(err,
			400),
	)
}
