package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunLogStream_FreeTier_Rejected proves that a Free-plan org cannot open
// the log-stream SSE route. The feature is gated by LogStreamingEnabled and
// resolved through checkFeatureAllowed.
func TestRunLogStream_FreeTier_Rejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: freeLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/stream/logs", "", "proj-1"))
	require.Equal(t, http.StatusForbidden,
		w.Code)
	assert.True(t,
		strings.Contains(w.Body.String(), "Log streaming"))

}

// TestRunLogStream_StarterTier_Allowed confirms the gate passes on the
// smallest tier that has LogStreamingEnabled = true. The handler proceeds far
// enough to fail later (no pubsub configured), but the gate itself does not
// reject.
func TestRunLogStream_StarterTier_Allowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
	}
	starter := billing.GetPlanLimits(domain.PlanStarter)
	enforcer := &tunableLimitsEnforcer{limits: starter}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/stream/logs", "", "proj-1"))
	require.NotEqual(t, http.
		StatusForbidden, w.Code,
	)

}

// TestRunLogStream_NilEnforcer_FailsOpen confirms community builds do not
// block log streaming.
func TestRunLogStream_NilEnforcer_FailsOpen(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID: id, JobID: "job-1", ProjectID: "proj-1",
				Status: domain.StatusExecuting, Attempt: 1,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.edition = domain.EditionCommunity

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/runs/run-1/stream/logs", "", "proj-1"))
	require.NotEqual(t, http.
		StatusForbidden, w.Code,
	)

}
