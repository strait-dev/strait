package api

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestTenantIso_OrgQueries_InternalSecret_RequiresOrgID rejects empty and
// non-UUID org_id parameters on the internal-secret listing path.
// Without this gate, a bug or typo could silently dispatch a wide-open
// store query against the unscoped path and leak rows.
func TestTenantIso_OrgQueries_InternalSecret_RequiresOrgID(t *testing.T) {
	t.Parallel()

	called := false
	ms := &APIStoreMock{
		ListRunsByOrgFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			called = true
			return nil, nil
		},
		ListJobsByOrgFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.Job, error) {
			called = true
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.Background()

	// Empty and non-UUID identifiers must reject.
	for _, badOrg := range []string{
		"",
		"org-1",
		"not-a-uuid",
		"00000000-0000-0000-0000",
	} {
		_, err := srv.handleListOrgRuns(ctx, &ListOrgRunsInput{OrgID: badOrg})
		require.Error(t, err)
		require.False(t, called)

		_, err = srv.handleListOrgJobs(ctx, &ListOrgJobsInput{OrgID: badOrg})
		require.Error(t, err)
		require.False(t, called)
	}
}

// TestTenantIso_OrgQueries_InternalSecret_AuditEmitted verifies that every
// internal-secret invocation against ListOrgRuns/ListOrgJobs writes a
// structured log line. No audit_event row is associated (no project_id),
// so the log is the canonical record.
func TestTenantIso_OrgQueries_InternalSecret_AuditEmitted(t *testing.T) {
	// Not parallel: we swap the process-wide default slog handler.
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ms := &APIStoreMock{
		ListRunsByOrgFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{}, nil
		},
		ListJobsByOrgFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.Job, error) {
			return []domain.Job{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	const orgUUID = "11111111-1111-4111-8111-111111111111"

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/organizations/"+orgUUID+"/runs", ""))
	require.Equal(t, http.
		StatusOK, w.
		Code)

	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, authedRequest(http.MethodGet, "/v1/organizations/"+orgUUID+"/jobs", ""))
	require.Equal(t, http.
		StatusOK, w2.
		Code)

	logs := buf.String()
	require.Contains(t, logs, "org_queries internal-secret listing")
	require.Contains(t, logs, "op=ListOrgRuns")
	require.Contains(t, logs, "op=ListOrgJobs")
	require.Contains(t, logs, "org_id="+
		orgUUID)
}
