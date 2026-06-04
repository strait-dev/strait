package api

import (
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/robfig/cron/v3"
)

func alignToExecutionWindow(requested *time.Time, now time.Time, expr, tz string) (*time.Time, error) {
	if tz == "" {
		tz = "UTC"
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, err
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(expr)
	if err != nil {
		return nil, err
	}

	reference := now
	if requested != nil && requested.After(reference) {
		reference = *requested
	}
	referenceLocal := reference.In(loc)

	if cronMatchesInstant(schedule, referenceLocal) {
		if requested != nil {
			ts := requested.UTC()
			return &ts, nil
		}
		return nil, nil //nolint:nilnil // nil signals "trigger now" with no explicit time.
	}

	next := schedule.Next(referenceLocal)
	nextUTC := next.UTC()
	return &nextUTC, nil
}

func cronMatchesInstant(schedule cron.Schedule, ts time.Time) bool {
	truncated := ts.Truncate(time.Minute)
	previousMinute := truncated.Add(-time.Minute)
	return schedule.Next(previousMinute).Equal(truncated)
}

func triggerScheduledAt(
	job *domain.Job,
	projectQuota *store.ProjectQuota,
	requested *time.Time,
	now time.Time,
) (*time.Time, error) {
	if job.ExecutionWindowCron == "" {
		return requested, nil
	}
	timezone := job.Timezone
	if timezone == "" && projectQuota != nil {
		timezone = projectQuota.Timezone
	}
	scheduledAt, err := alignToExecutionWindow(requested, now, job.ExecutionWindowCron, timezone)
	if err != nil {
		return nil, fmt.Errorf("execution window validation failed: %w", err)
	}
	return scheduledAt, nil
}

func (s *Server) triggerExpiresAt(job *domain.Job, req TriggerRequest, scheduledAt *time.Time, now time.Time) time.Time {
	expiresBase := triggerExpiryBase(now, scheduledAt)
	if req.TTLSecs != nil && *req.TTLSecs > 0 {
		return expiresBase.Add(time.Duration(*req.TTLSecs) * time.Second)
	}
	if job.RunTTLSecs > 0 {
		return expiresBase.Add(time.Duration(job.RunTTLSecs) * time.Second)
	}
	if s.config.DefaultRunTTLSecs > 0 {
		return expiresBase.Add(time.Duration(s.config.DefaultRunTTLSecs) * time.Second)
	}
	return expiresBase.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
}

func triggerInitialStatus(scheduledAt *time.Time, now time.Time) domain.RunStatus {
	if scheduledAt != nil && scheduledAt.After(now) {
		return domain.StatusDelayed
	}
	return domain.StatusQueued
}

func triggerExpiryBase(now time.Time, scheduledAt *time.Time) time.Time {
	if scheduledAt != nil && scheduledAt.After(now) {
		return *scheduledAt
	}
	return now
}
