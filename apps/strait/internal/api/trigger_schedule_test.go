package api

import (
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestTriggerInitialStatus(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Minute)
	past := now.Add(-time.Minute)
	require.Equal(t, domain.
		StatusDelayed, triggerInitialStatus(&future, now))
	require.Equal(t, domain.
		StatusQueued, triggerInitialStatus(&past, now))
	require.Equal(t, domain.
		StatusQueued, triggerInitialStatus(nil, now))
}

func TestTriggerExpiryBaseUsesFutureSchedule(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	if got := triggerExpiryBase(now, &future); !got.Equal(future) {
		require.Failf(t, "test failure",

			"future expiry base = %s, want %s", got, future)
	}
	if got := triggerExpiryBase(now, &past); !got.Equal(now) {
		require.Failf(t, "test failure",

			"past expiry base = %s, want %s", got, now)
	}
}

func TestTriggerExpiresAtPriority(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	scheduledAt := now.Add(time.Hour)
	ttlSecs := 30
	srv := &Server{config: &config.Config{DefaultRunTTLSecs: 90}}
	job := &domain.Job{TimeoutSecs: 10, RunTTLSecs: 60}

	got := srv.triggerExpiresAt(job, TriggerRequest{TTLSecs: &ttlSecs}, &scheduledAt, now)
	want := scheduledAt.Add(30 * time.Second)
	require.True(
		t, got.Equal(want))

	got = srv.triggerExpiresAt(job, TriggerRequest{}, &scheduledAt, now)
	want = scheduledAt.Add(60 * time.Second)
	require.True(
		t, got.Equal(want))

	job.RunTTLSecs = 0
	got = srv.triggerExpiresAt(job, TriggerRequest{}, &scheduledAt, now)
	want = scheduledAt.Add(90 * time.Second)
	require.True(
		t, got.Equal(want))

	srv.config.DefaultRunTTLSecs = 0
	got = srv.triggerExpiresAt(job, TriggerRequest{}, &scheduledAt, now)
	want = scheduledAt.Add(70 * time.Second)
	require.True(
		t, got.Equal(want))
}
