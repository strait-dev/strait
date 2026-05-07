//go:build !integration

package clickhouse

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------.
// Helpers
// ---------------------------------------------------------------------------.

// canceledCtx returns a context that is already canceled.
func canceledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// invertedRange returns from > to for invalid date range testing.
func invertedRange() (time.Time, time.Time) {
	to := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	from := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)
	return from, to
}

// shortRange returns a range less than 24 hours.
func shortRange() (time.Time, time.Time) {
	to := time.Date(2024, 1, 1, 6, 0, 0, 0, time.UTC)
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	return from, to
}

// longRange returns a range greater than 24 hours.
func longRange() (time.Time, time.Time) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)
	return from, to
}

// ---------------------------------------------------------------------------.
// GetPerformanceAnalytics
// ---------------------------------------------------------------------------.

func TestGetPerformanceAnalytics_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	_, err := s.GetPerformanceAnalytics(canceledCtx(), "proj-1", 24)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetPerformanceAnalytics_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	_, err := s.GetPerformanceAnalytics(context.Background(), "", 24)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetPerformanceAnalytics_ZeroPeriod(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	_, err := s.GetPerformanceAnalytics(context.Background(), "proj-1", 0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetPerformanceAnalytics_NegativePeriod(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	_, err := s.GetPerformanceAnalytics(context.Background(), "proj-1", -1)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetCostAnalytics
// ---------------------------------------------------------------------------.

func TestGetCostAnalytics_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostAnalytics(canceledCtx(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetCostAnalytics_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostAnalytics(context.Background(), "", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetCostAnalytics_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetCostAnalytics(context.Background(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetCostTrends
// ---------------------------------------------------------------------------.

func TestGetCostTrends_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostTrends(canceledCtx(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetCostTrends_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostTrends(context.Background(), "", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetCostTrends_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetCostTrends(context.Background(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetCostTrends_ShortPeriodBranch(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := shortRange()
	_, err := s.GetCostTrends(context.Background(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetCostTrends_LongPeriodBranch(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostTrends(context.Background(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetTopCosts
// ---------------------------------------------------------------------------.

func TestGetTopCosts_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopCosts(canceledCtx(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetTopCosts_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopCosts(context.Background(), "", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetTopCosts_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopCosts(context.Background(), "proj-1", from, to, 0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetTopCosts_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetTopCosts(context.Background(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetCostOutliers
// ---------------------------------------------------------------------------.

func TestGetCostOutliers_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostOutliers(canceledCtx(), "proj-1", from, to, 2.0)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetCostOutliers_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostOutliers(context.Background(), "", from, to, 2.0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetCostOutliers_ZeroThreshold(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostOutliers(context.Background(), "proj-1", from, to, 0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetCostOutliers_NegativeThreshold(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostOutliers(context.Background(), "proj-1", from, to, -1.0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetCostOutliers_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetCostOutliers(context.Background(), "proj-1", from, to, 2.0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetRunTimeline
// ---------------------------------------------------------------------------.

func TestGetRunTimeline_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunTimeline(canceledCtx(), "proj-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetRunTimeline_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunTimeline(context.Background(), "", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetRunTimeline_HourBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := shortRange()
	_, err := s.GetRunTimeline(context.Background(), "proj-1", from, to, "hour")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetRunTimeline_DayBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunTimeline(context.Background(), "proj-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetRunTimeline_UnknownBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	// Unknown bucket value should fall through to default (toStartOfDay).
	_, err := s.GetRunTimeline(context.Background(), "proj-1", from, to, "unknown")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetRunTimeline_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetRunTimeline(context.Background(), "proj-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetRunDurationDistribution
// ---------------------------------------------------------------------------.

func TestGetRunDurationDistribution_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunDurationDistribution(canceledCtx(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetRunDurationDistribution_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunDurationDistribution(context.Background(), "", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetRunDurationDistribution_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetRunDurationDistribution(context.Background(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetRunFailureReasons
// ---------------------------------------------------------------------------.

func TestGetRunFailureReasons_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunFailureReasons(canceledCtx(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetRunFailureReasons_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunFailureReasons(context.Background(), "", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetRunFailureReasons_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunFailureReasons(context.Background(), "proj-1", from, to, 0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetRunFailureReasons_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetRunFailureReasons(context.Background(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetRunsByTrigger
// ---------------------------------------------------------------------------.

func TestGetRunsByTrigger_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunsByTrigger(canceledCtx(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetRunsByTrigger_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunsByTrigger(context.Background(), "", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetRunsByTrigger_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetRunsByTrigger(context.Background(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetJobHistory
// ---------------------------------------------------------------------------.

func TestGetJobHistory_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobHistory(canceledCtx(), "proj-1", "job-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetJobHistory_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobHistory(context.Background(), "", "job-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetJobHistory_EmptyJobID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobHistory(context.Background(), "proj-1", "", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetJobHistory_HourBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := shortRange()
	_, err := s.GetJobHistory(context.Background(), "proj-1", "job-1", from, to, "hour")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetJobHistory_DayBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobHistory(context.Background(), "proj-1", "job-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetJobHistory_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetJobHistory(context.Background(), "proj-1", "job-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetJobComparison
// ---------------------------------------------------------------------------.

func TestGetJobComparison_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobComparison(canceledCtx(), "proj-1", []string{"j1", "j2"}, from, to)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetJobComparison_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobComparison(context.Background(), "", []string{"j1"}, from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetJobComparison_NilJobIDs(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobComparison(context.Background(), "proj-1", nil, from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetJobComparison_EmptyJobIDs(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobComparison(context.Background(), "proj-1", []string{}, from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetJobComparison_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetJobComparison(context.Background(), "proj-1", []string{"j1"}, from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetJobReliability
// ---------------------------------------------------------------------------.

func TestGetJobReliability_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobReliability(canceledCtx(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetJobReliability_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobReliability(context.Background(), "", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetJobReliability_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobReliability(context.Background(), "proj-1", from, to, 0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetJobReliability_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetJobReliability(context.Background(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetRunsByVersion
// ---------------------------------------------------------------------------.

func TestGetRunsByVersion_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunsByVersion(canceledCtx(), "proj-1", "job-1", from, to)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetRunsByVersion_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunsByVersion(context.Background(), "", "job-1", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetRunsByVersion_EmptyJobID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunsByVersion(context.Background(), "proj-1", "", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetRunsByVersion_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetRunsByVersion(context.Background(), "proj-1", "job-1", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetJobCostRanking
// ---------------------------------------------------------------------------.

func TestGetJobCostRanking_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobCostRanking(canceledCtx(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetJobCostRanking_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobCostRanking(context.Background(), "", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetJobCostRanking_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobCostRanking(context.Background(), "proj-1", from, to, 0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetJobCostRanking_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetJobCostRanking(context.Background(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetTopFailingJobs
// ---------------------------------------------------------------------------.

func TestGetTopFailingJobs_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingJobs(canceledCtx(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetTopFailingJobs_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingJobs(context.Background(), "", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetTopFailingJobs_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingJobs(context.Background(), "proj-1", from, to, 0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetTopFailingJobs_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetTopFailingJobs(context.Background(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetTagSummary
// ---------------------------------------------------------------------------.

func TestGetTagSummary_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTagSummary(canceledCtx(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetTagSummary_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTagSummary(context.Background(), "", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetTagSummary_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTagSummary(context.Background(), "proj-1", from, to, 0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetTagSummary_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetTagSummary(context.Background(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetTopFailingTags
// ---------------------------------------------------------------------------.

func TestGetTopFailingTags_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingTags(canceledCtx(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetTopFailingTags_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingTags(context.Background(), "", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetTopFailingTags_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingTags(context.Background(), "proj-1", from, to, 0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetTopFailingTags_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetTopFailingTags(context.Background(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetTagCost
// ---------------------------------------------------------------------------.

func TestGetTagCost_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTagCost(canceledCtx(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetTagCost_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTagCost(context.Background(), "", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetTagCost_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTagCost(context.Background(), "proj-1", from, to, 0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetTagCost_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetTagCost(context.Background(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetWorkflowStepDurations
// ---------------------------------------------------------------------------.

func TestGetWorkflowStepDurations_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWorkflowStepDurations(canceledCtx(), "proj-1", "wf-1", from, to)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetWorkflowStepDurations_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWorkflowStepDurations(context.Background(), "", "wf-1", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetWorkflowStepDurations_EmptyWorkflowID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWorkflowStepDurations(context.Background(), "proj-1", "", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetWorkflowStepDurations_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetWorkflowStepDurations(context.Background(), "proj-1", "wf-1", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetWorkflowCompletionRates
// ---------------------------------------------------------------------------.

func TestGetWorkflowCompletionRates_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWorkflowCompletionRates(canceledCtx(), "proj-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetWorkflowCompletionRates_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWorkflowCompletionRates(context.Background(), "", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetWorkflowCompletionRates_HourBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := shortRange()
	_, err := s.GetWorkflowCompletionRates(context.Background(), "proj-1", from, to, "hour")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetWorkflowCompletionRates_DayBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWorkflowCompletionRates(context.Background(), "proj-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetWorkflowCompletionRates_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetWorkflowCompletionRates(context.Background(), "proj-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetWebhookDeliveryStats
// ---------------------------------------------------------------------------.

func TestGetWebhookDeliveryStats_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWebhookDeliveryStats(canceledCtx(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetWebhookDeliveryStats_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWebhookDeliveryStats(context.Background(), "", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetWebhookDeliveryStats_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetWebhookDeliveryStats(context.Background(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetWebhookEndpointHealth
// ---------------------------------------------------------------------------.

func TestGetWebhookEndpointHealth_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWebhookEndpointHealth(canceledCtx(), "proj-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetWebhookEndpointHealth_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWebhookEndpointHealth(context.Background(), "", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetWebhookEndpointHealth_HourBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := shortRange()
	_, err := s.GetWebhookEndpointHealth(context.Background(), "proj-1", from, to, "hour")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetWebhookEndpointHealth_DayBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWebhookEndpointHealth(context.Background(), "proj-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetWebhookEndpointHealth_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetWebhookEndpointHealth(context.Background(), "proj-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetTopFailingWebhooks
// ---------------------------------------------------------------------------.

func TestGetTopFailingWebhooks_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingWebhooks(canceledCtx(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetTopFailingWebhooks_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingWebhooks(context.Background(), "", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetTopFailingWebhooks_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingWebhooks(context.Background(), "proj-1", from, to, 0)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetTopFailingWebhooks_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetTopFailingWebhooks(context.Background(), "proj-1", from, to, 10)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetEventVolume
// ---------------------------------------------------------------------------.

func TestGetEventVolume_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetEventVolume(canceledCtx(), "proj-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetEventVolume_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetEventVolume(context.Background(), "", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetEventVolume_HourBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := shortRange()
	_, err := s.GetEventVolume(context.Background(), "proj-1", from, to, "hour")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetEventVolume_DayBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetEventVolume(context.Background(), "proj-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetEventVolume_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetEventVolume(context.Background(), "proj-1", from, to, "day")
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// GetCostByTrigger
// ---------------------------------------------------------------------------.

func TestGetCostByTrigger_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostByTrigger(canceledCtx(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestGetCostByTrigger_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostByTrigger(context.Background(), "", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

func TestGetCostByTrigger_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetCostByTrigger(context.Background(), "proj-1", from, to)
	if err == nil {
		t.Fatal("expected error from closed db")
	}
}

// ---------------------------------------------------------------------------.
// Table-driven: verify every function returns an error containing its
// expected error substring when called with a closed-db client.
// This exercises every Query/QueryRow path with real error propagation.
// ---------------------------------------------------------------------------.

func TestAnalyticsCoverage_ClosedDB_ErrorMessages(t *testing.T) {
	t.Parallel()

	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	now := time.Now()
	from := now.Add(-24 * time.Hour)

	tests := []struct {
		name      string
		fn        func() error
		wantInErr string
	}{
		{"GetPerformanceAnalytics", func() error {
			_, err := s.GetPerformanceAnalytics(context.Background(), "p1", 24)
			return err
		}, "slowest jobs"},
		{"GetCostAnalytics", func() error {
			_, err := s.GetCostAnalytics(context.Background(), "p1", from, now)
			return err
		}, "cost analytics"},
		{"GetCostTrends_short", func() error {
			_, err := s.GetCostTrends(context.Background(), "p1", now.Add(-time.Hour), now)
			return err
		}, "cost trends"},
		{"GetCostTrends_long", func() error {
			_, err := s.GetCostTrends(context.Background(), "p1", from, now)
			return err
		}, "cost trends"},
		{"GetTopCosts", func() error {
			_, err := s.GetTopCosts(context.Background(), "p1", from, now, 10)
			return err
		}, "top costs"},
		{"GetCostOutliers", func() error {
			_, err := s.GetCostOutliers(context.Background(), "p1", from, now, 2.0)
			return err
		}, "cost outliers"},
		{"GetRunTimeline_hour", func() error {
			_, err := s.GetRunTimeline(context.Background(), "p1", from, now, "hour")
			return err
		}, "run timeline"},
		{"GetRunTimeline_day", func() error {
			_, err := s.GetRunTimeline(context.Background(), "p1", from, now, "day")
			return err
		}, "run timeline"},
		{"GetRunDurationDistribution", func() error {
			_, err := s.GetRunDurationDistribution(context.Background(), "p1", from, now)
			return err
		}, "duration distribution"},
		{"GetRunFailureReasons", func() error {
			_, err := s.GetRunFailureReasons(context.Background(), "p1", from, now, 10)
			return err
		}, "failure reasons"},
		{"GetRunsByTrigger", func() error {
			_, err := s.GetRunsByTrigger(context.Background(), "p1", from, now)
			return err
		}, "runs by trigger"},
		{"GetJobHistory_hour", func() error {
			_, err := s.GetJobHistory(context.Background(), "p1", "j1", from, now, "hour")
			return err
		}, "job history"},
		{"GetJobHistory_day", func() error {
			_, err := s.GetJobHistory(context.Background(), "p1", "j1", from, now, "day")
			return err
		}, "job history"},
		{"GetJobComparison", func() error {
			_, err := s.GetJobComparison(context.Background(), "p1", []string{"j1", "j2"}, from, now)
			return err
		}, "job comparison"},
		{"GetJobReliability", func() error {
			_, err := s.GetJobReliability(context.Background(), "p1", from, now, 10)
			return err
		}, "job reliability"},
		{"GetRunsByVersion", func() error {
			_, err := s.GetRunsByVersion(context.Background(), "p1", "j1", from, now)
			return err
		}, "runs by version"},
		{"GetJobCostRanking", func() error {
			_, err := s.GetJobCostRanking(context.Background(), "p1", from, now, 10)
			return err
		}, "job cost ranking"},
		{"GetTopFailingJobs", func() error {
			_, err := s.GetTopFailingJobs(context.Background(), "p1", from, now, 10)
			return err
		}, "top failing jobs"},
		{"GetTagSummary", func() error {
			_, err := s.GetTagSummary(context.Background(), "p1", from, now, 10)
			return err
		}, "tag summary"},
		{"GetTopFailingTags", func() error {
			_, err := s.GetTopFailingTags(context.Background(), "p1", from, now, 10)
			return err
		}, "top failing tags"},
		{"GetTagCost", func() error {
			_, err := s.GetTagCost(context.Background(), "p1", from, now, 10)
			return err
		}, "tag cost"},
		{"GetWorkflowStepDurations", func() error {
			_, err := s.GetWorkflowStepDurations(context.Background(), "p1", "wf1", from, now)
			return err
		}, "workflow step durations"},
		{"GetWorkflowCompletionRates_hour", func() error {
			_, err := s.GetWorkflowCompletionRates(context.Background(), "p1", from, now, "hour")
			return err
		}, "workflow completion rates"},
		{"GetWorkflowCompletionRates_day", func() error {
			_, err := s.GetWorkflowCompletionRates(context.Background(), "p1", from, now, "day")
			return err
		}, "workflow completion rates"},
		{"GetWebhookDeliveryStats", func() error {
			_, err := s.GetWebhookDeliveryStats(context.Background(), "p1", from, now)
			return err
		}, "webhook delivery stats"},
		{"GetWebhookEndpointHealth_hour", func() error {
			_, err := s.GetWebhookEndpointHealth(context.Background(), "p1", from, now, "hour")
			return err
		}, "webhook endpoint health"},
		{"GetWebhookEndpointHealth_day", func() error {
			_, err := s.GetWebhookEndpointHealth(context.Background(), "p1", from, now, "day")
			return err
		}, "webhook endpoint health"},
		{"GetTopFailingWebhooks", func() error {
			_, err := s.GetTopFailingWebhooks(context.Background(), "p1", from, now, 10)
			return err
		}, "top failing webhooks"},
		{"GetEventVolume_hour", func() error {
			_, err := s.GetEventVolume(context.Background(), "p1", from, now, "hour")
			return err
		}, "event volume"},
		{"GetEventVolume_day", func() error {
			_, err := s.GetEventVolume(context.Background(), "p1", from, now, "day")
			return err
		}, "event volume"},
		{"GetCostByTrigger", func() error {
			_, err := s.GetCostByTrigger(context.Background(), "p1", from, now)
			return err
		}, "cost by trigger"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.fn()
			if err == nil {
				t.Fatal("expected error from closed db")
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantInErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------.
// Table-driven: nil client exercises the Query nil-guard path for all
// Query-first methods (returns "client is nil" error without panic).
// ---------------------------------------------------------------------------.

func TestAnalyticsCoverage_NilClient_AllQueryMethods(t *testing.T) {
	t.Parallel()

	s := NewAnalyticsStore(nil, nil)
	now := time.Now()
	from := now.Add(-7 * 24 * time.Hour)

	tests := []struct {
		name string
		fn   func() error
	}{
		{"GetPerformanceAnalytics", func() error {
			_, err := s.GetPerformanceAnalytics(context.Background(), "p1", 24)
			return err
		}},
		{"GetCostTrends", func() error {
			_, err := s.GetCostTrends(context.Background(), "p1", from, now)
			return err
		}},
		{"GetTopCosts", func() error {
			_, err := s.GetTopCosts(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetCostOutliers", func() error {
			_, err := s.GetCostOutliers(context.Background(), "p1", from, now, 2.0)
			return err
		}},
		{"GetRunTimeline", func() error {
			_, err := s.GetRunTimeline(context.Background(), "p1", from, now, "day")
			return err
		}},
		{"GetRunDurationDistribution", func() error {
			_, err := s.GetRunDurationDistribution(context.Background(), "p1", from, now)
			return err
		}},
		{"GetRunFailureReasons", func() error {
			_, err := s.GetRunFailureReasons(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetRunsByTrigger", func() error {
			_, err := s.GetRunsByTrigger(context.Background(), "p1", from, now)
			return err
		}},
		{"GetJobHistory", func() error {
			_, err := s.GetJobHistory(context.Background(), "p1", "j1", from, now, "day")
			return err
		}},
		{"GetJobComparison", func() error {
			_, err := s.GetJobComparison(context.Background(), "p1", []string{"j1"}, from, now)
			return err
		}},
		{"GetJobReliability", func() error {
			_, err := s.GetJobReliability(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetRunsByVersion", func() error {
			_, err := s.GetRunsByVersion(context.Background(), "p1", "j1", from, now)
			return err
		}},
		{"GetJobCostRanking", func() error {
			_, err := s.GetJobCostRanking(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetTopFailingJobs", func() error {
			_, err := s.GetTopFailingJobs(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetTagSummary", func() error {
			_, err := s.GetTagSummary(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetTopFailingTags", func() error {
			_, err := s.GetTopFailingTags(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetTagCost", func() error {
			_, err := s.GetTagCost(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetWorkflowStepDurations", func() error {
			_, err := s.GetWorkflowStepDurations(context.Background(), "p1", "wf1", from, now)
			return err
		}},
		{"GetWorkflowCompletionRates", func() error {
			_, err := s.GetWorkflowCompletionRates(context.Background(), "p1", from, now, "day")
			return err
		}},
		{"GetWebhookDeliveryStats", func() error {
			_, err := s.GetWebhookDeliveryStats(context.Background(), "p1", from, now)
			return err
		}},
		{"GetWebhookEndpointHealth", func() error {
			_, err := s.GetWebhookEndpointHealth(context.Background(), "p1", from, now, "day")
			return err
		}},
		{"GetTopFailingWebhooks", func() error {
			_, err := s.GetTopFailingWebhooks(context.Background(), "p1", from, now, 10)
			return err
		}},
		{"GetEventVolume", func() error {
			_, err := s.GetEventVolume(context.Background(), "p1", from, now, "day")
			return err
		}},
		{"GetCostByTrigger", func() error {
			_, err := s.GetCostByTrigger(context.Background(), "p1", from, now)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.fn()
			if err == nil {
				t.Fatal("expected error from nil client")
			}
		})
	}
}
