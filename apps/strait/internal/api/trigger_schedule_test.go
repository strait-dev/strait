package api

import (
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
)

func TestTriggerInitialStatus(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Minute)
	past := now.Add(-time.Minute)

	if got := triggerInitialStatus(&future, now); got != domain.StatusDelayed {
		t.Fatalf("future status = %s, want delayed", got)
	}
	if got := triggerInitialStatus(&past, now); got != domain.StatusQueued {
		t.Fatalf("past status = %s, want queued", got)
	}
	if got := triggerInitialStatus(nil, now); got != domain.StatusQueued {
		t.Fatalf("nil status = %s, want queued", got)
	}
}

func TestTriggerExpiryBaseUsesFutureSchedule(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	if got := triggerExpiryBase(now, &future); !got.Equal(future) {
		t.Fatalf("future expiry base = %s, want %s", got, future)
	}
	if got := triggerExpiryBase(now, &past); !got.Equal(now) {
		t.Fatalf("past expiry base = %s, want %s", got, now)
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
	if !got.Equal(want) {
		t.Fatalf("request ttl expires_at = %s, want %s", got, want)
	}

	got = srv.triggerExpiresAt(job, TriggerRequest{}, &scheduledAt, now)
	want = scheduledAt.Add(60 * time.Second)
	if !got.Equal(want) {
		t.Fatalf("job ttl expires_at = %s, want %s", got, want)
	}

	job.RunTTLSecs = 0
	got = srv.triggerExpiresAt(job, TriggerRequest{}, &scheduledAt, now)
	want = scheduledAt.Add(90 * time.Second)
	if !got.Equal(want) {
		t.Fatalf("default ttl expires_at = %s, want %s", got, want)
	}

	srv.config.DefaultRunTTLSecs = 0
	got = srv.triggerExpiresAt(job, TriggerRequest{}, &scheduledAt, now)
	want = scheduledAt.Add(70 * time.Second)
	if !got.Equal(want) {
		t.Fatalf("timeout fallback expires_at = %s, want %s", got, want)
	}
}
