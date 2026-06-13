package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestTriggerIdempotencyKeyPrefersPrimaryHeader(t *testing.T) {
	t.Parallel()

	key, err := triggerIdempotencyKey(&TriggerJobInput{
		XIdempotencyKey:   "primary-key",
		IdempotencyKeyAlt: "standard-key",
	})
	require.NoError(t, err)
	require.Equal(t, "primary-key",
		key,
	)
}

func TestTriggerIdempotencyKeyFallsBackToStandardHeader(t *testing.T) {
	t.Parallel()

	key, err := triggerIdempotencyKey(&TriggerJobInput{IdempotencyKeyAlt: "standard-key"})
	require.NoError(t, err)
	require.Equal(t, "standard-key",
		key,
	)
}

func TestTriggerIdempotencyKeyRejectsTooLong(t *testing.T) {
	t.Parallel()

	_, err := triggerIdempotencyKey(&TriggerJobInput{
		XIdempotencyKey: strings.Repeat("x", maxIdempotencyKeyLength+1),
	})
	require.Error(t, err)
	require.Contains(
		t, err.
			Error(), "idempotency key must be 256 characters or fewer")
}

func TestTriggerIdempotencyHitReturnsExistingRun(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			require.Equal(t, "job-1",
				jobID)
			require.Equal(t, "idem-key",
				key)

			return &domain.JobRun{ID: "run-existing", Status: domain.StatusQueued}, nil
		},
	}}

	hit, err := srv.triggerIdempotencyHit(context.Background(), &domain.Job{ID: "job-1"}, "idem-key")
	require.NoError(t, err)

	assertIdempotencyResponse(t, hit, "run-existing", domain.StatusQueued)
}

func TestResolveTriggerIdempotencyConflictReturnsWinningRun(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			require.Equal(t, "job-1",
				jobID)
			require.Equal(t, "idem-key",
				key)

			return &domain.JobRun{ID: "run-winner", Status: domain.StatusExecuting}, nil
		},
	}}

	err := srv.resolveTriggerIdempotencyConflict(
		context.Background(),
		&domain.Job{ID: "job-1"},
		"idem-key",
		domain.ErrIdempotencyConflict,
	)

	var statusErr *rawStatusError
	require.ErrorAs(
		t, err, &statusErr)

	assertIdempotencyResponse(t, statusErr, "run-winner", domain.StatusExecuting)
}

func assertIdempotencyResponse(t *testing.T, err *rawStatusError, runID string, status domain.RunStatus) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, http.StatusOK,
		err.
			status)

	body, ok := err.body.(triggerIdempotencyResponse)
	require.True(
		t, ok)
	require.Equal(t, runID,
		body.ID)
	require.Equal(t, status,
		body.Status)
	require.True(t, body.IdempotencyHit)
}
