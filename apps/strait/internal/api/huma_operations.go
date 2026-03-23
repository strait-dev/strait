package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

// registerHumaOperations registers all API operations on the Huma API
// for OpenAPI documentation generation. These operations run on a separate
// chi router and do not handle actual requests -- they only generate the
// OpenAPI specification that Scalar serves at /reference.
func (s *Server) registerHumaOperations(api huma.API) {
	s.registerHealthOps(api)
	s.registerPlanOps(api)
	s.registerJobOps(api)
	s.registerRunOps(api)
	s.registerProjectOps(api)
}

// registerHealthOps registers health check operations.

type HealthCheckOutput struct {
	Body struct {
		Status  string `json:"status" example:"ok" doc:"Service health status"`
		Edition string `json:"edition,omitempty" example:"cloud" doc:"Service edition"`
	}
}

func (s *Server) registerHealthOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "health-check",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Health check",
		Description: "Returns the service health status.",
		Tags:        []string{"Health"},
	}, func(_ context.Context, _ *struct{}) (*HealthCheckOutput, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "health-ready",
		Method:      http.MethodGet,
		Path:        "/health/ready",
		Summary:     "Readiness check",
		Description: "Returns 200 when the service is ready to accept traffic.",
		Tags:        []string{"Health"},
	}, func(_ context.Context, _ *struct{}) (*HealthCheckOutput, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerPlanOps registers plan listing operations.

type ListPlansOutput struct {
	Body struct {
		Plans []planResponse `json:"plans" doc:"Available plan tiers"`
	}
}

func (s *Server) registerPlanOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-plans",
		Method:      http.MethodGet,
		Path:        "/v1/plans",
		Summary:     "List plan tiers",
		Description: "Returns all available plan tiers with their limits and pricing.",
		Tags:        []string{"Plans"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(_ context.Context, _ *struct{}) (*ListPlansOutput, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerJobOps registers job CRUD and trigger operations.

type CreateJobBody struct {
	Name        string          `json:"name" required:"true" minLength:"1" maxLength:"255" doc:"Job name" example:"daily-report"`
	Slug        string          `json:"slug,omitempty" doc:"URL-friendly identifier"`
	EndpointURL string          `json:"endpoint_url" required:"true" doc:"HTTP endpoint to call" example:"https://api.example.com/webhook"`
	Cron        string          `json:"cron,omitempty" doc:"Cron expression" example:"0 9 * * *"`
	Payload     json.RawMessage `json:"payload,omitempty" doc:"Default JSON payload"`
	MaxAttempts int             `json:"max_attempts,omitempty" minimum:"1" maximum:"10" doc:"Max retry attempts" example:"3"`
	TimeoutSecs int             `json:"timeout_secs,omitempty" minimum:"1" maximum:"86400" doc:"Timeout in seconds" example:"300"`
	WebhookURL  string          `json:"webhook_url,omitempty" doc:"Webhook URL for notifications"`
	Tags        json.RawMessage `json:"tags,omitempty" doc:"Key-value tags"`
	Enabled     *bool           `json:"enabled,omitempty" doc:"Whether job is enabled"`
}

type JobResponseBody struct {
	Body domain.Job
}

type TriggerJobBody struct {
	Payload        json.RawMessage `json:"payload,omitempty" doc:"Custom payload for this run"`
	IdempotencyKey string          `json:"idempotency_key,omitempty" doc:"Prevent duplicate triggers"`
	ScheduledFor   *time.Time      `json:"scheduled_for,omitempty" doc:"Schedule for future execution"`
}

func (s *Server) registerJobOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-job",
		Method:      http.MethodPost,
		Path:        "/v1/jobs",
		Summary:     "Create a job",
		Description: "Creates a new job with the specified endpoint and configuration.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(_ context.Context, _ *struct{ Body CreateJobBody }) (*JobResponseBody, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-jobs",
		Method:      http.MethodGet,
		Path:        "/v1/jobs",
		Summary:     "List jobs",
		Description: "Returns a paginated list of jobs in the current project.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor"`
	}) (*struct{ Body []domain.Job }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-job",
		Method:      http.MethodGet,
		Path:        "/v1/jobs/{jobID}",
		Summary:     "Get a job",
		Description: "Returns details of a specific job.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID"`
	}) (*JobResponseBody, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "trigger-job",
		Method:      http.MethodPost,
		Path:        "/v1/jobs/{jobID}/trigger",
		Summary:     "Trigger a job",
		Description: "Triggers immediate execution of a job.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(_ context.Context, _ *struct {
		JobID string         `path:"jobID" doc:"Job ID"`
		Body  TriggerJobBody `json:"body"`
	}) (*struct{ Body domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-job",
		Method:      http.MethodPatch,
		Path:        "/v1/jobs/{jobID}",
		Summary:     "Update a job",
		Description: "Updates an existing job's configuration.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID"`
		Body  CreateJobBody
	}) (*JobResponseBody, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-job",
		Method:      http.MethodDelete,
		Path:        "/v1/jobs/{jobID}",
		Summary:     "Delete a job",
		Description: "Permanently deletes a job and all its associated data.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerRunOps registers run management operations.

func (s *Server) registerRunOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-run",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}",
		Summary:     "Get a run",
		Description: "Returns details of a specific job run.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID"`
	}) (*struct{ Body domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-runs",
		Method:      http.MethodGet,
		Path:        "/v1/runs",
		Summary:     "List runs",
		Description: "Returns a paginated list of job runs.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results"`
		Cursor string `query:"cursor" doc:"Pagination cursor"`
		Status string `query:"status" doc:"Filter by status" enum:"queued,executing,completed,failed,canceled"`
		JobID  string `query:"job_id" doc:"Filter by job ID"`
	}) (*struct{ Body []domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "cancel-run",
		Method:      http.MethodDelete,
		Path:        "/v1/runs/{runID}",
		Summary:     "Cancel a run",
		Description: "Cancels a queued or executing run.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "replay-run",
		Method:      http.MethodPost,
		Path:        "/v1/runs/{runID}/replay",
		Summary:     "Replay a run",
		Description: "Creates a new run with the same configuration as the original.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID"`
	}) (*struct{ Body domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerProjectOps registers project management operations.

func (s *Server) registerProjectOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-project",
		Method:      http.MethodPost,
		Path:        "/v1/projects",
		Summary:     "Create a project",
		Description: "Creates a new project within an organization.",
		Tags:        []string{"Projects"},
		Security:    []map[string][]string{{"internalSecret": {}}},
	}, func(_ context.Context, _ *struct {
		Body struct {
			ID    string `json:"id" required:"true" doc:"Project ID"`
			OrgID string `json:"org_id" required:"true" doc:"Organization ID"`
			Name  string `json:"name" required:"true" minLength:"2" doc:"Project name"`
		}
	}) (*struct{ Body domain.Project }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}
