//go:build !integration

package clickhouse

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helpers

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

// GetPerformanceAnalytics

func TestGetPerformanceAnalytics_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	_, err := s.GetPerformanceAnalytics(canceledCtx(), "proj-1", 24)
	require.Error(t, err)

}

func TestGetPerformanceAnalytics_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	_, err := s.GetPerformanceAnalytics(context.Background(), "", 24)
	require.Error(t, err)

}

func TestGetPerformanceAnalytics_ZeroPeriod(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	_, err := s.GetPerformanceAnalytics(context.Background(), "proj-1", 0)
	require.Error(t, err)

}

func TestGetPerformanceAnalytics_NegativePeriod(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	_, err := s.GetPerformanceAnalytics(context.Background(), "proj-1", -1)
	require.Error(t, err)

}

// GetCostAnalytics

func TestGetCostAnalytics_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostAnalytics(canceledCtx(), "proj-1", from, to)
	require.Error(t, err)

}

func TestGetCostAnalytics_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostAnalytics(context.Background(), "", from, to)
	require.Error(t, err)

}

func TestGetCostAnalytics_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetCostAnalytics(context.Background(), "proj-1", from, to)
	require.Error(t, err)

}

// GetCostTrends

func TestGetCostTrends_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostTrends(canceledCtx(), "proj-1", from, to)
	require.Error(t, err)

}

func TestGetCostTrends_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostTrends(context.Background(), "", from, to)
	require.Error(t, err)

}

func TestGetCostTrends_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetCostTrends(context.Background(), "proj-1", from, to)
	require.Error(t, err)

}

func TestGetCostTrends_ShortPeriodBranch(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := shortRange()
	_, err := s.GetCostTrends(context.Background(), "proj-1", from, to)
	require.Error(t, err)

}

func TestGetCostTrends_LongPeriodBranch(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostTrends(context.Background(), "proj-1", from, to)
	require.Error(t, err)

}

// GetTopCosts

func TestGetTopCosts_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopCosts(canceledCtx(), "proj-1", from, to, 10)
	require.Error(t, err)

}

func TestGetTopCosts_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopCosts(context.Background(), "", from, to, 10)
	require.Error(t, err)

}

func TestGetTopCosts_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopCosts(context.Background(), "proj-1", from, to, 0)
	require.Error(t, err)

}

func TestGetTopCosts_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetTopCosts(context.Background(), "proj-1", from, to, 10)
	require.Error(t, err)

}

// GetCostOutliers

func TestGetCostOutliers_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostOutliers(canceledCtx(), "proj-1", from, to, 2.0)
	require.Error(t, err)

}

func TestGetCostOutliers_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostOutliers(context.Background(), "", from, to, 2.0)
	require.Error(t, err)

}

func TestGetCostOutliers_ZeroThreshold(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostOutliers(context.Background(), "proj-1", from, to, 0)
	require.Error(t, err)

}

func TestGetCostOutliers_NegativeThreshold(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostOutliers(context.Background(), "proj-1", from, to, -1.0)
	require.Error(t, err)

}

func TestGetCostOutliers_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetCostOutliers(context.Background(), "proj-1", from, to, 2.0)
	require.Error(t, err)

}

// GetRunTimeline

func TestGetRunTimeline_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunTimeline(canceledCtx(), "proj-1", from, to, "day")
	require.Error(t, err)

}

func TestGetRunTimeline_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunTimeline(context.Background(), "", from, to, "day")
	require.Error(t, err)

}

func TestGetRunTimeline_HourBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := shortRange()
	_, err := s.GetRunTimeline(context.Background(), "proj-1", from, to, "hour")
	require.Error(t, err)

}

func TestGetRunTimeline_DayBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunTimeline(context.Background(), "proj-1", from, to, "day")
	require.Error(t, err)

}

func TestGetRunTimeline_UnknownBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	// Unknown bucket value should fall through to default (toStartOfDay).
	_, err := s.GetRunTimeline(context.Background(), "proj-1", from, to, "unknown")
	require.Error(t, err)

}

func TestGetRunTimeline_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetRunTimeline(context.Background(), "proj-1", from, to, "day")
	require.Error(t, err)

}

// GetRunDurationDistribution

func TestGetRunDurationDistribution_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunDurationDistribution(canceledCtx(), "proj-1", from, to)
	require.Error(t, err)

}

func TestGetRunDurationDistribution_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunDurationDistribution(context.Background(), "", from, to)
	require.Error(t, err)

}

func TestGetRunDurationDistribution_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetRunDurationDistribution(context.Background(), "proj-1", from, to)
	require.Error(t, err)

}

// GetRunFailureReasons

func TestGetRunFailureReasons_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunFailureReasons(canceledCtx(), "proj-1", from, to, 10)
	require.Error(t, err)

}

func TestGetRunFailureReasons_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunFailureReasons(context.Background(), "", from, to, 10)
	require.Error(t, err)

}

func TestGetRunFailureReasons_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunFailureReasons(context.Background(), "proj-1", from, to, 0)
	require.Error(t, err)

}

func TestGetRunFailureReasons_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetRunFailureReasons(context.Background(), "proj-1", from, to, 10)
	require.Error(t, err)

}

// GetRunsByTrigger

func TestGetRunsByTrigger_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunsByTrigger(canceledCtx(), "proj-1", from, to)
	require.Error(t, err)

}

func TestGetRunsByTrigger_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunsByTrigger(context.Background(), "", from, to)
	require.Error(t, err)

}

func TestGetRunsByTrigger_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetRunsByTrigger(context.Background(), "proj-1", from, to)
	require.Error(t, err)

}

// GetJobHistory

func TestGetJobHistory_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobHistory(canceledCtx(), "proj-1", "job-1", from, to, "day")
	require.Error(t, err)

}

func TestGetJobHistory_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobHistory(context.Background(), "", "job-1", from, to, "day")
	require.Error(t, err)

}

func TestGetJobHistory_EmptyJobID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobHistory(context.Background(), "proj-1", "", from, to, "day")
	require.Error(t, err)

}

func TestGetJobHistory_HourBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := shortRange()
	_, err := s.GetJobHistory(context.Background(), "proj-1", "job-1", from, to, "hour")
	require.Error(t, err)

}

func TestGetJobHistory_DayBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobHistory(context.Background(), "proj-1", "job-1", from, to, "day")
	require.Error(t, err)

}

func TestGetJobHistory_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetJobHistory(context.Background(), "proj-1", "job-1", from, to, "day")
	require.Error(t, err)

}

// GetJobComparison

func TestGetJobComparison_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobComparison(canceledCtx(), "proj-1", []string{"j1", "j2"}, from, to)
	require.Error(t, err)

}

func TestGetJobComparison_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobComparison(context.Background(), "", []string{"j1"}, from, to)
	require.Error(t, err)

}

func TestGetJobComparison_NilJobIDs(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobComparison(context.Background(), "proj-1", nil, from, to)
	require.Error(t, err)

}

func TestGetJobComparison_EmptyJobIDs(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobComparison(context.Background(), "proj-1", []string{}, from, to)
	require.Error(t, err)

}

func TestGetJobComparison_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetJobComparison(context.Background(), "proj-1", []string{"j1"}, from, to)
	require.Error(t, err)

}

// GetJobReliability

func TestGetJobReliability_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobReliability(canceledCtx(), "proj-1", from, to, 10)
	require.Error(t, err)

}

func TestGetJobReliability_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobReliability(context.Background(), "", from, to, 10)
	require.Error(t, err)

}

func TestGetJobReliability_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobReliability(context.Background(), "proj-1", from, to, 0)
	require.Error(t, err)

}

func TestGetJobReliability_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetJobReliability(context.Background(), "proj-1", from, to, 10)
	require.Error(t, err)

}

// GetRunsByVersion

func TestGetRunsByVersion_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunsByVersion(canceledCtx(), "proj-1", "job-1", from, to)
	require.Error(t, err)

}

func TestGetRunsByVersion_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunsByVersion(context.Background(), "", "job-1", from, to)
	require.Error(t, err)

}

func TestGetRunsByVersion_EmptyJobID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetRunsByVersion(context.Background(), "proj-1", "", from, to)
	require.Error(t, err)

}

func TestGetRunsByVersion_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetRunsByVersion(context.Background(), "proj-1", "job-1", from, to)
	require.Error(t, err)

}

// GetJobCostRanking

func TestGetJobCostRanking_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobCostRanking(canceledCtx(), "proj-1", from, to, 10)
	require.Error(t, err)

}

func TestGetJobCostRanking_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobCostRanking(context.Background(), "", from, to, 10)
	require.Error(t, err)

}

func TestGetJobCostRanking_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetJobCostRanking(context.Background(), "proj-1", from, to, 0)
	require.Error(t, err)

}

func TestGetJobCostRanking_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetJobCostRanking(context.Background(), "proj-1", from, to, 10)
	require.Error(t, err)

}

// GetTopFailingJobs

func TestGetTopFailingJobs_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingJobs(canceledCtx(), "proj-1", from, to, 10)
	require.Error(t, err)

}

func TestGetTopFailingJobs_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingJobs(context.Background(), "", from, to, 10)
	require.Error(t, err)

}

func TestGetTopFailingJobs_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingJobs(context.Background(), "proj-1", from, to, 0)
	require.Error(t, err)

}

func TestGetTopFailingJobs_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetTopFailingJobs(context.Background(), "proj-1", from, to, 10)
	require.Error(t, err)

}

// GetTagSummary

func TestGetTagSummary_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTagSummary(canceledCtx(), "proj-1", from, to, 10)
	require.Error(t, err)

}

func TestGetTagSummary_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTagSummary(context.Background(), "", from, to, 10)
	require.Error(t, err)

}

func TestGetTagSummary_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTagSummary(context.Background(), "proj-1", from, to, 0)
	require.Error(t, err)

}

func TestGetTagSummary_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetTagSummary(context.Background(), "proj-1", from, to, 10)
	require.Error(t, err)

}

// GetTopFailingTags

func TestGetTopFailingTags_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingTags(canceledCtx(), "proj-1", from, to, 10)
	require.Error(t, err)

}

func TestGetTopFailingTags_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingTags(context.Background(), "", from, to, 10)
	require.Error(t, err)

}

func TestGetTopFailingTags_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingTags(context.Background(), "proj-1", from, to, 0)
	require.Error(t, err)

}

func TestGetTopFailingTags_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetTopFailingTags(context.Background(), "proj-1", from, to, 10)
	require.Error(t, err)

}

// GetTagCost

func TestGetTagCost_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTagCost(canceledCtx(), "proj-1", from, to, 10)
	require.Error(t, err)

}

func TestGetTagCost_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTagCost(context.Background(), "", from, to, 10)
	require.Error(t, err)

}

func TestGetTagCost_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTagCost(context.Background(), "proj-1", from, to, 0)
	require.Error(t, err)

}

func TestGetTagCost_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetTagCost(context.Background(), "proj-1", from, to, 10)
	require.Error(t, err)

}

// GetWorkflowStepDurations

func TestGetWorkflowStepDurations_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWorkflowStepDurations(canceledCtx(), "proj-1", "wf-1", from, to)
	require.Error(t, err)

}

func TestGetWorkflowStepDurations_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWorkflowStepDurations(context.Background(), "", "wf-1", from, to)
	require.Error(t, err)

}

func TestGetWorkflowStepDurations_EmptyWorkflowID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWorkflowStepDurations(context.Background(), "proj-1", "", from, to)
	require.Error(t, err)

}

func TestGetWorkflowStepDurations_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetWorkflowStepDurations(context.Background(), "proj-1", "wf-1", from, to)
	require.Error(t, err)

}

// GetWorkflowCompletionRates

func TestGetWorkflowCompletionRates_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWorkflowCompletionRates(canceledCtx(), "proj-1", from, to, "day")
	require.Error(t, err)

}

func TestGetWorkflowCompletionRates_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWorkflowCompletionRates(context.Background(), "", from, to, "day")
	require.Error(t, err)

}

func TestGetWorkflowCompletionRates_HourBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := shortRange()
	_, err := s.GetWorkflowCompletionRates(context.Background(), "proj-1", from, to, "hour")
	require.Error(t, err)

}

func TestGetWorkflowCompletionRates_DayBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWorkflowCompletionRates(context.Background(), "proj-1", from, to, "day")
	require.Error(t, err)

}

func TestGetWorkflowCompletionRates_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetWorkflowCompletionRates(context.Background(), "proj-1", from, to, "day")
	require.Error(t, err)

}

// GetWebhookDeliveryStats

func TestGetWebhookDeliveryStats_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWebhookDeliveryStats(canceledCtx(), "proj-1", from, to)
	require.Error(t, err)

}

func TestGetWebhookDeliveryStats_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWebhookDeliveryStats(context.Background(), "", from, to)
	require.Error(t, err)

}

func TestGetWebhookDeliveryStats_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetWebhookDeliveryStats(context.Background(), "proj-1", from, to)
	require.Error(t, err)

}

// GetWebhookEndpointHealth

func TestGetWebhookEndpointHealth_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWebhookEndpointHealth(canceledCtx(), "proj-1", from, to, "day")
	require.Error(t, err)

}

func TestGetWebhookEndpointHealth_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWebhookEndpointHealth(context.Background(), "", from, to, "day")
	require.Error(t, err)

}

func TestGetWebhookEndpointHealth_HourBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := shortRange()
	_, err := s.GetWebhookEndpointHealth(context.Background(), "proj-1", from, to, "hour")
	require.Error(t, err)

}

func TestGetWebhookEndpointHealth_DayBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetWebhookEndpointHealth(context.Background(), "proj-1", from, to, "day")
	require.Error(t, err)

}

func TestGetWebhookEndpointHealth_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetWebhookEndpointHealth(context.Background(), "proj-1", from, to, "day")
	require.Error(t, err)

}

// GetTopFailingWebhooks

func TestGetTopFailingWebhooks_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingWebhooks(canceledCtx(), "proj-1", from, to, 10)
	require.Error(t, err)

}

func TestGetTopFailingWebhooks_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingWebhooks(context.Background(), "", from, to, 10)
	require.Error(t, err)

}

func TestGetTopFailingWebhooks_ZeroLimit(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetTopFailingWebhooks(context.Background(), "proj-1", from, to, 0)
	require.Error(t, err)

}

func TestGetTopFailingWebhooks_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetTopFailingWebhooks(context.Background(), "proj-1", from, to, 10)
	require.Error(t, err)

}

// GetEventVolume

func TestGetEventVolume_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetEventVolume(canceledCtx(), "proj-1", from, to, "day")
	require.Error(t, err)

}

func TestGetEventVolume_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetEventVolume(context.Background(), "", from, to, "day")
	require.Error(t, err)

}

func TestGetEventVolume_HourBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := shortRange()
	_, err := s.GetEventVolume(context.Background(), "proj-1", from, to, "hour")
	require.Error(t, err)

}

func TestGetEventVolume_DayBucket(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetEventVolume(context.Background(), "proj-1", from, to, "day")
	require.Error(t, err)

}

func TestGetEventVolume_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetEventVolume(context.Background(), "proj-1", from, to, "day")
	require.Error(t, err)

}

// GetCostByTrigger

func TestGetCostByTrigger_CanceledCtx(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostByTrigger(canceledCtx(), "proj-1", from, to)
	require.Error(t, err)

}

func TestGetCostByTrigger_EmptyProjectID(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := longRange()
	_, err := s.GetCostByTrigger(context.Background(), "", from, to)
	require.Error(t, err)

}

func TestGetCostByTrigger_InvertedRange(t *testing.T) {
	t.Parallel()
	client := newClosedDBClient(t)
	s := NewAnalyticsStore(client, nil)
	from, to := invertedRange()
	_, err := s.GetCostByTrigger(context.Background(), "proj-1", from, to)
	require.Error(t, err)

}

// Table-driven: verify every function returns an error containing its
// expected error substring when called with a closed-db client.
// This exercises every Query/QueryRow path with real error propagation.

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
			require.Error(t, err)
			assert.True(t, strings.Contains(err.
				Error(),
				tt.wantInErr,
			))

		})
	}
}

// Table-driven: nil client exercises the Query nil-guard path for all
// Query-first methods (returns "client is nil" error without panic).

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
			require.Error(t, err)

		})
	}
}
