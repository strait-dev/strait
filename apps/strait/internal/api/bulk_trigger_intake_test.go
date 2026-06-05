package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestValidateBulkTriggerRequestRejectsTooManyItems(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	items := make([]BulkTriggerItem, srv.config.MaxBulkTriggerItems+1)

	err := srv.validateBulkTriggerRequest(BulkTriggerRequest{Items: items})
	assertStatusError(t, err, http.StatusBadRequest, "maximum 500 items")
}

func TestValidateBulkTriggerItemReportsItemIndex(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	past := time.Now().Add(-time.Minute)

	err := srv.validateBulkTriggerItem(&domain.Job{ID: "job-1"}, BulkTriggerItem{ScheduledAt: &past}, 7)
	assertStatusError(t, err, http.StatusBadRequest, "item 7")
}

func TestValidateBulkTriggerItemAppliesPayloadSchema(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	job := &domain.Job{
		ID:            "job-1",
		PayloadSchema: json.RawMessage(`{"type":"object","required":["customer"]}`),
	}

	err := srv.validateBulkTriggerItem(job, BulkTriggerItem{Payload: json.RawMessage(`{"order":"1"}`)}, 2)
	assertStatusError(t, err, http.StatusBadRequest, "payload validation failed for item 2")
}

func TestBulkHasIdempotencyKey(t *testing.T) {
	t.Parallel()
	require.False(t, bulkHasIdempotencyKey([]BulkTriggerItem{{}, {Payload: json.RawMessage(`{}`)}}))
	require.True(t, bulkHasIdempotencyKey([]BulkTriggerItem{{}, {IdempotencyKey: "k"}}))
}
