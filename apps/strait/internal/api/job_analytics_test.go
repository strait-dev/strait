package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/store"
)

func TestHandleJobHistory_Success(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetJobHistoryFunc: func(_ context.Context, _, jobID string, _, _ time.Time, bucket string) ([]store.JobHistoryBucket, error) {
			if jobID != "job-1" {
				t.Fatalf("expected job-1, got %s", jobID)
			}
			if bucket != "day" {
				t.Fatalf("expected default bucket 'day', got %q", bucket)
			}
			return []store.JobHistoryBucket{
				{Period: "2026-01-01T00:00:00Z", Completed: 5, Failed: 1, AvgDurationMs: 1000, P95DurationMs: 3000},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("jobs/job-1/history", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleJobHistory_MissingParams(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", "/v1/analytics/jobs/job-1/history", "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleJobHistory_StoreError(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetJobHistoryFunc: func(_ context.Context, _, _ string, _, _ time.Time, _ string) ([]store.JobHistoryBucket, error) {
			return nil, errors.New("db error")
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("jobs/job-1/history", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleJobComparison_Success(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetJobComparisonFunc: func(_ context.Context, _ string, jobIDs []string, _, _ time.Time) ([]store.JobComparison, error) {
			if len(jobIDs) != 2 {
				t.Fatalf("expected 2 job IDs, got %d", len(jobIDs))
			}
			return []store.JobComparison{
				{JobID: "job-1", Slug: "my-job", Total: 100, SuccessRate: 0.95, AvgDurationMs: 1200, Cost: 50000},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("jobs/comparison", validFrom(), validTo(), "job_ids", "job-1,job-2"), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleJobComparison_MissingJobIDs(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("jobs/comparison", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleJobReliability_Success(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetJobReliabilityFunc: func(_ context.Context, _ string, _, _ time.Time, limit int) ([]store.JobReliability, error) {
			if limit != 10 {
				t.Fatalf("expected default limit 10, got %d", limit)
			}
			return []store.JobReliability{
				{JobID: "job-1", Slug: "flaky-job", Total: 100, SuccessRate: 0.5, Failed: 50},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("jobs/reliability", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRunsByVersion_Success(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetRunsByVersionFunc: func(_ context.Context, _, jobID string, _, _ time.Time) ([]store.RunsByVersion, error) {
			if jobID != "job-1" {
				t.Fatalf("expected job-1, got %s", jobID)
			}
			return []store.RunsByVersion{
				{VersionID: "v1", Total: 50, Completed: 48, Failed: 2, AvgDurationMs: 1200},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("jobs/by-version", validFrom(), validTo(), "job_id", "job-1"), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRunsByVersion_MissingJobID(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("jobs/by-version", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleJobCostRanking_Success(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetJobCostRankingFunc: func(_ context.Context, _ string, _, _ time.Time, _ int) ([]store.JobCostRanking, error) {
			return []store.JobCostRanking{
				{JobID: "job-1", Slug: "expensive", TotalCost: 100000, RunCount: 10, AvgCostPerRun: 10000},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("jobs/cost-ranking", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result []store.JobCostRanking
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result) != 1 || result[0].TotalCost != 100000 {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestHandleTopFailingJobs_Success(t *testing.T) {
	t.Parallel()
	as := &AnalyticsStoreMock{
		GetTopFailingJobsFunc: func(_ context.Context, _ string, _, _ time.Time, _ int) ([]store.TopFailingJob, error) {
			return []store.TopFailingJob{
				{JobID: "job-1", Slug: "bad-job", FailedCount: 30, Total: 100, FailureRate: 0.3},
			}, nil
		},
	}
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, as, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("jobs/top-failing", validFrom(), validTo()), "", "proj-1"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTopFailingJobs_InvalidLimit(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithAnalytics(t, &APIStoreMock{}, &AnalyticsStoreMock{}, &mockQueue{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest("GET", analyticsURL("jobs/top-failing", validFrom(), validTo(), "limit", "abc"), "", "proj-1"))
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
