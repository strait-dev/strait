package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

// DryRunValidationResult contains the result of trigger validation for dry-run mode.
type DryRunValidationResult struct {
	Job                *DryRunJobInfo  `json:"job"`
	PayloadHash        string          `json:"payload_hash"`
	Payload            json.RawMessage `json:"payload,omitempty"`
	ScheduledAt        *time.Time      `json:"scheduled_at,omitempty"`
	ExpiresAt          time.Time       `json:"expires_at"`
	ValidationWarnings []string        `json:"validation_warnings,omitempty"`
}

type DryRunJobInfo struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	Slug          string               `json:"slug"`
	ExecutionMode domain.ExecutionMode `json:"execution_mode"`
	Queue         string               `json:"queue,omitempty"`
	TimeoutSecs   int                  `json:"timeout_secs"`
	MaxAttempts   int                  `json:"max_attempts"`
	Version       int                  `json:"version"`
	VersionID     string               `json:"version_id,omitempty"`
}

func (s *Server) handleTriggerDryRun(ctx context.Context, jobID string, req TriggerRequest) (*TriggerJobOutput, error) {
	result, err := s.validateTriggerRequest(ctx, jobID, req)
	if err != nil {
		var statusErr huma.StatusError
		if errors.As(err, &statusErr) {
			return nil, statusErr
		}
		return nil, huma.Error400BadRequest(err.Error())
	}
	return nil, &rawStatusError{status: http.StatusOK, body: result}
}

func (s *Server) validateTriggerRequest(ctx context.Context, jobID string, req TriggerRequest) (*DryRunValidationResult, error) {
	if err := validateRunCreationJobID(jobID); err != nil {
		return nil, err
	}
	if err := validatePayloadSize(req.Payload); err != nil {
		return nil, err
	}

	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if !job.Enabled {
		return nil, errors.New("job is disabled")
	}

	if job.Paused {
		return nil, errors.New("job is paused -- resume it before triggering new runs")
	}

	if err := validateTriggerScheduledAt(req.ScheduledAt); err != nil {
		return nil, err
	}

	if err := validatePayloadAgainstSchema(req.Payload, job.PayloadSchema); err != nil {
		return nil, fmt.Errorf("payload validation failed: %w", err)
	}

	payload, payloadHash, err := canonicalizePayload(req.Payload)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	projectQuota, err := s.quotaCache.Get(ctx, job.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("load project quota: %w", err)
	}

	if err := s.validateDryRunProjectQuota(ctx, job, projectQuota); err != nil {
		return nil, err
	}

	if err := s.checkTriggerDispatchPriority(ctx, job.ProjectID, req.Priority); err != nil {
		return nil, err
	}

	if err := s.validateDryRunJobRateLimit(ctx, job); err != nil {
		return nil, err
	}

	warnings, err := s.dryRunValidationWarnings(ctx, job, payload)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	scheduledAt, err := triggerScheduledAt(job, projectQuota, req.ScheduledAt, now)
	if err != nil {
		return nil, err
	}
	expiresAt := s.triggerExpiresAt(job, req, scheduledAt, now)

	return &DryRunValidationResult{
		Job:                dryRunJobInfo(job),
		PayloadHash:        payloadHash,
		Payload:            payload,
		ScheduledAt:        scheduledAt,
		ExpiresAt:          expiresAt,
		ValidationWarnings: warnings,
	}, nil
}

func (s *Server) validateDryRunProjectQuota(ctx context.Context, job *domain.Job, projectQuota *store.ProjectQuota) error {
	if projectQuota == nil {
		return nil
	}
	if err := s.checkTriggerDailyCostBudget(ctx, job.ProjectID, projectQuota); err != nil {
		return err
	}
	if projectQuota.MaxQueuedRuns > 0 {
		queuedRuns, err := s.store.CountProjectQueuedRuns(ctx, job.ProjectID)
		if err != nil {
			return fmt.Errorf("evaluate project queued quota: %w", err)
		}
		if queuedRuns >= projectQuota.MaxQueuedRuns {
			return errors.New("project queued quota exceeded")
		}
	}
	if projectQuota.MaxExecutingRuns > 0 {
		activeRuns, err := s.store.CountProjectActiveRuns(ctx, job.ProjectID)
		if err != nil {
			return fmt.Errorf("evaluate project active quota: %w", err)
		}
		if activeRuns >= projectQuota.MaxExecutingRuns {
			return errors.New("project executing quota exceeded")
		}
	}
	return nil
}

func (s *Server) validateDryRunJobRateLimit(ctx context.Context, job *domain.Job) error {
	if job.RateLimitMax <= 0 || job.RateLimitWindowSecs <= 0 {
		return nil
	}
	since := time.Now().Add(-time.Duration(job.RateLimitWindowSecs) * time.Second)
	runCount, err := s.store.CountRunsForJobSince(ctx, job.ID, since)
	if err != nil {
		return fmt.Errorf("evaluate job rate limit: %w", err)
	}
	if runCount >= job.RateLimitMax {
		return errors.New("job rate limit exceeded")
	}
	return nil
}

func (s *Server) dryRunValidationWarnings(ctx context.Context, job *domain.Job, payload json.RawMessage) ([]string, error) {
	warnings := []string{}
	if job.DedupWindowSecs <= 0 {
		return warnings, nil
	}
	since := time.Now().Add(-time.Duration(job.DedupWindowSecs) * time.Second)
	existingRun, err := s.store.FindRecentRunByPayload(ctx, job.ID, payload, since)
	if err != nil {
		return nil, fmt.Errorf("evaluate payload deduplication: %w", err)
	}
	if existingRun != nil {
		warnings = append(warnings, fmt.Sprintf("payload deduplication: run %s", existingRun.ID))
	}
	return warnings, nil
}

func dryRunJobInfo(job *domain.Job) *DryRunJobInfo {
	if job == nil {
		return nil
	}
	return &DryRunJobInfo{
		ID:            job.ID,
		Name:          job.Name,
		Slug:          job.Slug,
		ExecutionMode: job.ExecutionMode,
		Queue:         job.Queue,
		TimeoutSecs:   job.TimeoutSecs,
		MaxAttempts:   job.MaxAttempts,
		Version:       job.Version,
		VersionID:     job.VersionID,
	}
}
