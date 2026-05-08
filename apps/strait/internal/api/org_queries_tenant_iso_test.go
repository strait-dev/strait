package api

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
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
		if err == nil {
			t.Fatalf("expected error for non-uuid org_id %q", badOrg)
		}
		if called {
			t.Fatalf("store must not be called for non-uuid org_id %q", badOrg)
		}
		_, err = srv.handleListOrgJobs(ctx, &ListOrgJobsInput{OrgID: badOrg})
		if err == nil {
			t.Fatalf("expected error for non-uuid org_id %q (jobs)", badOrg)
		}
		if called {
			t.Fatalf("store must not be called for non-uuid org_id %q (jobs)", badOrg)
		}
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
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from runs list, got %d: %s", w.Code, w.Body.String())
	}
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, authedRequest(http.MethodGet, "/v1/organizations/"+orgUUID+"/jobs", ""))
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 from jobs list, got %d: %s", w2.Code, w2.Body.String())
	}

	logs := buf.String()
	if !strings.Contains(logs, "org_queries internal-secret listing") {
		t.Fatalf("expected internal-secret audit log line, got: %s", logs)
	}
	if !strings.Contains(logs, "op=ListOrgRuns") {
		t.Fatalf("expected ListOrgRuns op log, got: %s", logs)
	}
	if !strings.Contains(logs, "op=ListOrgJobs") {
		t.Fatalf("expected ListOrgJobs op log, got: %s", logs)
	}
	if !strings.Contains(logs, "org_id="+orgUUID) {
		t.Fatalf("expected org_id field in log, got: %s", logs)
	}
}
