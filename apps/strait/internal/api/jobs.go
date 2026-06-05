package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/billing"
	"strait/internal/clickhouse"
	"strait/internal/domain"
	"strait/internal/store"
)

const defaultJobQueueName = "default"

type UpdateJobRequest struct {
	Name                      *string            `json:"name,omitempty"`
	Slug                      *string            `json:"slug,omitempty"`
	GroupID                   *string            `json:"group_id,omitempty"`
	Description               *string            `json:"description,omitempty"`
	Cron                      *string            `json:"cron,omitempty"`
	PayloadSchema             *json.RawMessage   `json:"payload_schema,omitempty"`
	Tags                      *map[string]string `json:"tags,omitempty"`
	EndpointURL               *string            `json:"endpoint_url,omitempty" validate:"omitempty,url"`
	EndpointSigningSecret     *string            `json:"endpoint_signing_secret,omitempty" validate:"omitempty,min=16,max=4096"`
	WebhookSecret             *string            `json:"webhook_secret,omitempty" validate:"omitempty,min=16,max=4096" doc:"Alias of endpoint_signing_secret used by the Go SDK. When both are set, webhook_secret wins and a warning is logged."`
	FallbackEndpointURL       *string            `json:"fallback_endpoint_url,omitempty" validate:"omitempty,url"`
	MaxAttempts               *int               `json:"max_attempts,omitempty" validate:"omitempty,min=1,max=100"`
	TimeoutSecs               *int               `json:"timeout_secs,omitempty" validate:"omitempty,min=1"`
	MaxConcurrency            *int               `json:"max_concurrency,omitempty" validate:"omitempty,min=0"`
	MaxConcurrencyPerKey      *int               `json:"max_concurrency_per_key,omitempty" validate:"omitempty,min=0"`
	ExecutionWindowCron       *string            `json:"execution_window_cron,omitempty"`
	Timezone                  *string            `json:"timezone,omitempty"`
	RateLimitMax              *int               `json:"rate_limit_max,omitempty" validate:"omitempty,min=0"`
	RateLimitWindowSecs       *int               `json:"rate_limit_window_secs,omitempty" validate:"omitempty,min=0"`
	DedupWindowSecs           *int               `json:"dedup_window_secs,omitempty" validate:"omitempty,min=0"`
	RunTTLSecs                *int               `json:"run_ttl_secs,omitempty" validate:"omitempty,min=0"`
	RetryStrategy             *string            `json:"retry_strategy,omitempty" validate:"omitempty,oneof=exponential linear fixed custom"`
	RetryDelaysSecs           *[]int             `json:"retry_delays_secs,omitempty"`
	RetryPriorityBoost        *int               `json:"retry_priority_boost,omitempty" validate:"omitempty,min=0,max=10"`
	EnvironmentID             *string            `json:"environment_id,omitempty"`
	Enabled                   *bool              `json:"enabled,omitempty"`
	VersionPolicy             *string            `json:"version_policy,omitempty" validate:"omitempty,oneof=pin latest minor"`
	BackwardsCompatible       *bool              `json:"backwards_compatible,omitempty"`
	DefaultRunMetadata        *map[string]string `json:"default_run_metadata,omitempty"`
	ResultSchema              *json.RawMessage   `json:"result_schema,omitempty"`
	CronOverlapPolicy         *string            `json:"cron_overlap_policy,omitempty" validate:"omitempty,oneof=allow skip cancel_running"`
	DebounceWindowSecs        *int               `json:"debounce_window_secs,omitempty" validate:"omitempty,min=0"`
	BatchWindowSecs           *int               `json:"batch_window_secs,omitempty" validate:"omitempty,min=0"`
	BatchMaxSize              *int               `json:"batch_max_size,omitempty" validate:"omitempty,min=0"`
	ExecutionMode             *string            `json:"execution_mode,omitempty" validate:"omitempty,oneof=http worker"`
	QueueName                 *string            `json:"queue_name,omitempty"`
	PoisonPillThreshold       *int               `json:"poison_pill_threshold,omitempty" validate:"omitempty,min=1" doc:"Consecutive identical errors before auto-quarantine to DLQ. NULL or 0 disables."`
	OnCompleteTriggerWorkflow *string            `json:"on_complete_trigger_workflow,omitempty"`
	OnCompleteTriggerJob      *string            `json:"on_complete_trigger_job,omitempty"`
	OnCompletePayloadMapping  *json.RawMessage   `json:"on_complete_payload_mapping,omitempty"`
	OnFailureTriggerJob       *string            `json:"on_failure_trigger_job,omitempty"`
	OnFailureTriggerWorkflow  *string            `json:"on_failure_trigger_workflow,omitempty"`
	OnFailurePayloadMapping   *json.RawMessage   `json:"on_failure_payload_mapping,omitempty"`
}

// GetJobInput is the typed input for getting a single job.
type GetJobInput struct {
	JobID string `path:"jobID"`
}

// GetJobOutput is the typed output for getting a single job.
type GetJobOutput struct {
	Body *domain.Job
}

func (s *Server) handleGetJob(ctx context.Context, input *GetJobInput) (*GetJobOutput, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if job == nil {
		return nil, huma.Error404NotFound("job not found")
	}

	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}

	return &GetJobOutput{Body: job}, nil
}

// ListJobsInput is the typed input for listing jobs.
type ListJobsInput struct {
	Slug     string `query:"slug"`
	TagKey   string `query:"tag_key"`
	TagValue string `query:"tag_value"`
	Limit    string `query:"limit"`
	Cursor   string `query:"cursor"`
}

// ListJobsOutput is the typed output for listing jobs.
type ListJobsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListJobs(ctx context.Context, input *ListJobsInput) (*ListJobsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.TagValue != "" && input.TagKey == "" {
		return nil, huma.Error400BadRequest("tag_key is required when tag_value is provided")
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	// Slug lookup: return a single-item list when ?slug= is provided.
	if input.Slug != "" {
		emptyPage := func() *ListJobsOutput {
			return &ListJobsOutput{Body: paginatedResult([]domain.Job{}, limit, func(j domain.Job) string {
				return j.CreatedAt.Format(time.RFC3339Nano)
			})}
		}
		job, jobErr := s.store.GetJobBySlug(ctx, projectID, input.Slug)
		if jobErr != nil {
			if errors.Is(jobErr, store.ErrJobNotFound) {
				return emptyPage(), nil
			}
			return nil, huma.Error500InternalServerError("failed to look up job by slug")
		}
		if callerEnv := environmentIDFromContext(ctx); callerEnv != "" && job.EnvironmentID != callerEnv {
			return emptyPage(), nil
		}
		return &ListJobsOutput{Body: paginatedResult([]domain.Job{*job}, limit, func(j domain.Job) string {
			return j.CreatedAt.Format(time.RFC3339Nano)
		})}, nil
	}

	var (
		jobs    []domain.Job
		listErr error
	)
	if input.TagKey != "" {
		jobs, listErr = s.store.ListJobsByTag(ctx, projectID, input.TagKey, input.TagValue, limit+1, cursor)
	} else {
		jobs, listErr = s.store.ListJobs(ctx, projectID, limit+1, cursor)
	}
	if listErr != nil {
		return nil, huma.Error500InternalServerError("failed to list jobs")
	}
	jobs = filterJobsForEnvironment(ctx, jobs)

	return &ListJobsOutput{Body: paginatedResult(jobs, limit, func(j domain.Job) string {
		return j.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

// enqueueJobMetadata sends a job metadata record to the ClickHouse exporter
// so the job_metadata table stays in sync with Postgres.
func (s *Server) enqueueJobMetadata(job *domain.Job) {
	if s.chExporter == nil || job == nil {
		return
	}
	s.chExporter.Enqueue(clickhouse.JobMetadataRecord{
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Slug:      job.Slug,
	})
}

// DeleteJobInput is the typed input for deleting a job.
type DeleteJobInput struct {
	JobID string `path:"jobID"`
}

func (s *Server) handleDeleteJob(ctx context.Context, input *DeleteJobInput) (*struct{}, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}

	if err := s.store.DeleteJob(ctx, input.JobID); err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		if errors.Is(err, store.ErrJobHasActiveRuns) {
			return nil, huma.Error409Conflict("job has active runs — cancel them first or wait for completion")
		}
		return nil, huma.Error500InternalServerError("failed to delete job")
	}
	s.invalidateWorkerJobCache(ctx, input.JobID, time.Now().UnixNano())

	slog.Info("job deleted",
		"job_id", input.JobID,
		"actor", actorFromContext(ctx),
		"project_id", projectIDFromContext(ctx))
	s.emitAuditEvent(ctx, domain.AuditActionJobDeleted, "job", input.JobID, nil)

	return nil, nil
}

func filterJobsForEnvironment(ctx context.Context, jobs []domain.Job) []domain.Job {
	callerEnv := environmentIDFromContext(ctx)
	if callerEnv == "" {
		return jobs
	}
	filtered := jobs[:0]
	for _, job := range jobs {
		if job.EnvironmentID == callerEnv {
			filtered = append(filtered, job)
		}
	}
	return filtered
}

// JobHealthResponse wraps health stats with the time window.
type JobHealthResponse struct {
	JobID  string    `json:"job_id"`
	Window string    `json:"window"`
	Since  time.Time `json:"since"`
	*store.JobHealthStats
}

// GetJobHealthInput is the typed input for getting job health stats.
type GetJobHealthInput struct {
	JobID  string `path:"jobID"`
	Window string `query:"window"`
}

// GetJobHealthOutput is the typed output for getting job health stats.
type GetJobHealthOutput struct {
	Body JobHealthResponse
}

func (s *Server) handleGetJobHealth(ctx context.Context, input *GetJobHealthInput) (*GetJobHealthOutput, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}

	window := input.Window
	var since time.Time
	switch window {
	case "1h":
		since = time.Now().Add(-time.Hour)
	case "1d":
		since = time.Now().Add(-24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	case "7d", "":
		window = "7d"
		since = time.Now().Add(-7 * 24 * time.Hour)
	default:
		return nil, huma.Error400BadRequest("invalid window: must be 1h, 1d, 7d, or 30d")
	}

	stats, err := s.store.GetJobHealthStats(ctx, input.JobID, since)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to compute health stats")
	}

	return &GetJobHealthOutput{Body: JobHealthResponse{
		JobID:          input.JobID,
		Window:         window,
		Since:          since,
		JobHealthStats: stats,
	}}, nil
}

// checkHTTPModeAllowed verifies that HTTP execution mode is allowed for the org's plan.
// Returns nil if allowed, or a 400 error if the plan doesn't support HTTP mode.
func (s *Server) checkHTTPModeAllowed(ctx context.Context, mode domain.ExecutionMode, projectID string) error {
	if mode != domain.ExecutionModeHTTP {
		return nil
	}
	if !s.edition.RequiresHTTPModeGating() {
		return nil
	}
	if s.billingEnforcer == nil {
		return planGateUnavailable("http_mode_enforcer", errors.New("billing enforcer not configured"))
	}

	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		if err != nil {
			return planGateUnavailable("http_mode_org_lookup", err)
		}
		return nil
	}

	limits, err := s.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		return planGateUnavailable("http_mode_plan_lookup", err)
	}

	if !limits.AllowsHTTPMode {
		billing.RecordHTTPModeGateRejected(ctx, string(limits.PlanTier), "job_create")
		return huma.Error400BadRequest("HTTP execution mode is unavailable for this organization. Contact support if this persists.")
	}
	return nil
}
