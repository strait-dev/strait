// NOTE: This file contains all Huma operation registrations. Consider splitting
// into domain-specific files (huma_ops_jobs.go, huma_ops_runs.go, etc.) if it
// becomes difficult to navigate.
package api

import (
	"context"
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
	s.registerWorkflowOps(api)
	s.registerWorkflowRunOps(api)
	s.registerDeploymentOps(api)
	s.registerEventOps(api)
	s.registerEventSourceOps(api)
	s.registerWebhookOps(api)
	s.registerAPIKeyOps(api)
	s.registerCLIAuthOps(api)
	s.registerSecretOps(api)
	s.registerLogDrainOps(api)
	s.registerNotificationOps(api)
	s.registerRBACOps(api)
	s.registerAuditOps(api)
	s.registerBillingOps(api)
	s.registerReferralOps(api)
	s.registerAnalyticsOps(api)
	s.registerSDKOps(api)
	s.registerOrgQueryOps(api)
	s.registerBatchOperationOps(api)
	s.registerJobGroupOps(api)
	s.registerEnvironmentOps(api)
	s.registerJobExtrasOps(api)
	s.registerRunExtrasOps(api)
	s.registerRegionOps(api)
	s.registerStatsOps(api)
	s.registerWorkflowPolicyOps(api)
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
		Errors:      []int{401, 500},
	}, func(_ context.Context, _ *struct{}) (*ListPlansOutput, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerJobOps registers job CRUD and trigger operations.

type CreateJobBody struct {
	Name        string `json:"name" required:"true" minLength:"1" maxLength:"255" doc:"Job name" example:"daily-report"`
	Slug        string `json:"slug,omitempty" doc:"URL-friendly identifier" example:"daily-report-sync"`
	EndpointURL string `json:"endpoint_url" required:"true" doc:"HTTP endpoint to call" example:"https://api.example.com/webhooks/strait"`
	Cron        string `json:"cron,omitempty" doc:"Cron expression" example:"0 9 * * *"`
	Payload     any    `json:"payload,omitempty" doc:"Arbitrary JSON payload"`
	MaxAttempts int    `json:"max_attempts,omitempty" minimum:"1" maximum:"10" doc:"Max retry attempts" example:"3"`
	TimeoutSecs int    `json:"timeout_secs,omitempty" minimum:"1" maximum:"86400" doc:"Timeout in seconds" example:"300"`
	WebhookURL  string `json:"webhook_url,omitempty" doc:"Webhook URL for notifications" example:"https://api.example.com/webhooks/notify"`
	Tags        any    `json:"tags,omitempty" doc:"Key-value metadata tags"`
	Enabled     *bool  `json:"enabled,omitempty" doc:"Whether job is enabled" example:"true"`
}

type JobResponseBody struct {
	Body domain.Job
}

type TriggerJobBody struct {
	Payload        any        `json:"payload,omitempty" doc:"Arbitrary JSON payload for this run"`
	IdempotencyKey string     `json:"idempotency_key,omitempty" doc:"Prevent duplicate triggers" example:"trigger-2024-01-15-report"`
	ScheduledFor   *time.Time `json:"scheduled_for,omitempty" doc:"Schedule for future execution" example:"2024-01-15T09:00:00Z"`
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
		Errors:      []int{400, 401, 409, 429, 500},
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
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
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
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID" example:"job_01HX7YJKM3"`
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
		Errors:      []int{400, 401, 404, 409, 429, 500},
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
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID" example:"job_01HX7YJKM3"`
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
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID" example:"job_01HX7YJKM3"`
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
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
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
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
		Status string `query:"status" doc:"Filter by status" enum:"queued,executing,completed,failed,canceled" example:"completed"`
		JobID  string `query:"job_id" doc:"Filter by job ID" example:"job_01HX7YJKM3"`
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
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
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
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
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
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			ID    string `json:"id" required:"true" doc:"Project ID" example:"proj_01HX7ZKRN5"`
			OrgID string `json:"org_id" required:"true" doc:"Organization ID" example:"org_01HX7YMWQ3"`
			Name  string `json:"name" required:"true" minLength:"2" doc:"Project name" example:"Payment Processor"`
		}
	}) (*struct{ Body domain.Project }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-projects",
		Method:      http.MethodGet,
		Path:        "/v1/projects",
		Summary:     "List projects",
		Description: "Returns all projects accessible by the current API key.",
		Tags:        []string{"Projects"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.Project }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-project",
		Method:      http.MethodGet,
		Path:        "/v1/projects/{projectID}",
		Summary:     "Get a project",
		Description: "Returns details of a specific project.",
		Tags:        []string{"Projects"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		ProjectID string `path:"projectID" doc:"Project ID" example:"proj_01HX7ZKRN5"`
	}) (*struct{ Body domain.Project }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-project",
		Method:      http.MethodDelete,
		Path:        "/v1/projects/{projectID}",
		Summary:     "Delete a project",
		Description: "Permanently deletes a project and all its associated resources.",
		Tags:        []string{"Projects"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		ProjectID string `path:"projectID" doc:"Project ID" example:"proj_01HX7ZKRN5"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-project-settings",
		Method:      http.MethodGet,
		Path:        "/v1/projects/{projectID}/settings",
		Summary:     "Get project settings",
		Description: "Returns the current settings for a project.",
		Tags:        []string{"Projects"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		ProjectID string `path:"projectID" doc:"Project ID" example:"proj_01HX7ZKRN5"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-project-settings",
		Method:      http.MethodPut,
		Path:        "/v1/projects/{projectID}/settings",
		Summary:     "Update project settings",
		Description: "Updates the settings for a project.",
		Tags:        []string{"Projects"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		ProjectID string `path:"projectID" doc:"Project ID" example:"proj_01HX7ZKRN5"`
		Body      any
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerWorkflowOps registers workflow CRUD and management operations.

func (s *Server) registerWorkflowOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-workflow",
		Method:      http.MethodPost,
		Path:        "/v1/workflows",
		Summary:     "Create a workflow",
		Description: "Creates a new workflow with step definitions and trigger configuration.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body domain.Workflow }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-workflows",
		Method:      http.MethodGet,
		Path:        "/v1/workflows",
		Summary:     "List workflows",
		Description: "Returns a paginated list of workflows in the current project.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.Workflow }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-workflow",
		Method:      http.MethodGet,
		Path:        "/v1/workflows/{workflowID}",
		Summary:     "Get a workflow",
		Description: "Returns details of a specific workflow including its step definitions.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
	}) (*struct{ Body domain.Workflow }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-workflow",
		Method:      http.MethodPatch,
		Path:        "/v1/workflows/{workflowID}",
		Summary:     "Update a workflow",
		Description: "Updates an existing workflow's configuration and step definitions.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
		Body       any
	}) (*struct{ Body domain.Workflow }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-workflow",
		Method:      http.MethodDelete,
		Path:        "/v1/workflows/{workflowID}",
		Summary:     "Delete a workflow",
		Description: "Permanently deletes a workflow and all its associated runs.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "dry-run-workflow",
		Method:      http.MethodPost,
		Path:        "/v1/workflows/{workflowID}/dry-run",
		Summary:     "Dry-run a workflow",
		Description: "Validates a workflow execution without creating actual runs.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
		Body       any
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "plan-workflow",
		Method:      http.MethodPost,
		Path:        "/v1/workflows/{workflowID}/plan",
		Summary:     "Plan a workflow execution",
		Description: "Generates an execution plan showing which steps will run and in what order.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
		Body       any
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "simulate-workflow",
		Method:      http.MethodPost,
		Path:        "/v1/workflows/{workflowID}/simulate",
		Summary:     "Simulate a workflow execution",
		Description: "Simulates a workflow run with mock data to preview step outcomes.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
		Body       any
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-workflow-graph",
		Method:      http.MethodGet,
		Path:        "/v1/workflows/{workflowID}/graph",
		Summary:     "Get workflow graph",
		Description: "Returns the DAG representation of the workflow's step dependencies.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "trigger-workflow",
		Method:      http.MethodPost,
		Path:        "/v1/workflows/{workflowID}/trigger",
		Summary:     "Trigger a workflow",
		Description: "Triggers a new execution of the workflow.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
		Body       any
	}) (*struct{ Body domain.WorkflowRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "clone-workflow",
		Method:      http.MethodPost,
		Path:        "/v1/workflows/{workflowID}/clone",
		Summary:     "Clone a workflow",
		Description: "Creates a copy of an existing workflow with a new name.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
		Body       any
	}) (*struct{ Body domain.Workflow }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-workflow-runs-by-workflow",
		Method:      http.MethodGet,
		Path:        "/v1/workflows/{workflowID}/runs",
		Summary:     "List runs for a workflow",
		Description: "Returns a paginated list of runs for a specific workflow.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
		Limit      int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor     string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.WorkflowRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-workflow-versions",
		Method:      http.MethodGet,
		Path:        "/v1/workflows/{workflowID}/versions",
		Summary:     "List workflow versions",
		Description: "Returns all versions of a workflow definition.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
	}) (*struct{ Body []domain.WorkflowVersion }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-workflow-version",
		Method:      http.MethodGet,
		Path:        "/v1/workflows/{workflowID}/versions/{versionID}",
		Summary:     "Get a workflow version",
		Description: "Returns details of a specific workflow version.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
		VersionID  string `path:"versionID" doc:"Version ID" example:"ver_01HX9FGTP2"`
	}) (*struct{ Body domain.WorkflowVersion }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-workflow-version-steps",
		Method:      http.MethodGet,
		Path:        "/v1/workflows/{workflowID}/versions/{versionID}/steps",
		Summary:     "List workflow version steps",
		Description: "Returns the step definitions for a specific workflow version.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
		VersionID  string `path:"versionID" doc:"Version ID" example:"ver_01HX9FGTP2"`
	}) (*struct{ Body []domain.WorkflowStep }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "diff-workflow-versions",
		Method:      http.MethodGet,
		Path:        "/v1/workflows/{workflowID}/versions/{fromVersionID}/diff/{toVersionID}",
		Summary:     "Diff workflow versions",
		Description: "Returns the differences between two workflow versions.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID    string `path:"workflowID" doc:"Workflow ID"`
		FromVersionID string `path:"fromVersionID" doc:"Source version ID" example:"ver_01HX9FGTP2"`
		ToVersionID   string `path:"toVersionID" doc:"Target version ID" example:"ver_01HX9GHTQ3"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-workflow-version-impact",
		Method:      http.MethodGet,
		Path:        "/v1/workflows/{workflowID}/versions/{versionID}/impact",
		Summary:     "Get workflow version impact",
		Description: "Returns the impact analysis for a specific workflow version.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
		VersionID  string `path:"versionID" doc:"Version ID" example:"ver_01HX9FGTP2"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-active-versions",
		Method:      http.MethodGet,
		Path:        "/v1/workflows/{workflowID}/active-versions",
		Summary:     "Get active workflow versions",
		Description: "Returns the currently active versions for a workflow, including traffic splits.",
		Tags:        []string{"Workflows"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerWorkflowRunOps registers workflow run management operations.

func (s *Server) registerWorkflowRunOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-workflow-runs",
		Method:      http.MethodGet,
		Path:        "/v1/workflow-runs",
		Summary:     "List workflow runs",
		Description: "Returns a paginated list of workflow runs in the current project.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.WorkflowRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "bulk-cancel-workflow-runs",
		Method:      http.MethodPost,
		Path:        "/v1/workflow-runs/bulk-cancel",
		Summary:     "Bulk cancel workflow runs",
		Description: "Cancels multiple workflow runs matching the provided filters.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			RunIDs []string `json:"run_ids" required:"true" doc:"List of workflow run IDs to cancel"`
		}
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "bulk-replay-workflow-runs",
		Method:      http.MethodPost,
		Path:        "/v1/workflow-runs/bulk-replay",
		Summary:     "Bulk replay workflow runs",
		Description: "Replays multiple workflow runs matching the provided filters.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			RunIDs []string `json:"run_ids" required:"true" doc:"List of workflow run IDs to replay"`
		}
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-workflow-run",
		Method:      http.MethodGet,
		Path:        "/v1/workflow-runs/{workflowRunID}",
		Summary:     "Get a workflow run",
		Description: "Returns details of a specific workflow run including step statuses.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
	}) (*struct{ Body domain.WorkflowRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "cancel-workflow-run",
		Method:      http.MethodDelete,
		Path:        "/v1/workflow-runs/{workflowRunID}",
		Summary:     "Cancel a workflow run",
		Description: "Cancels an active workflow run and all its pending steps.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "pause-workflow-run",
		Method:      http.MethodPost,
		Path:        "/v1/workflow-runs/{workflowRunID}/pause",
		Summary:     "Pause a workflow run",
		Description: "Pauses an active workflow run, preventing further steps from executing.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
	}) (*struct{ Body domain.WorkflowRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "resume-workflow-run",
		Method:      http.MethodPost,
		Path:        "/v1/workflow-runs/{workflowRunID}/resume",
		Summary:     "Resume a workflow run",
		Description: "Resumes a paused workflow run.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
	}) (*struct{ Body domain.WorkflowRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-workflow-run-labels",
		Method:      http.MethodGet,
		Path:        "/v1/workflow-runs/{workflowRunID}/labels",
		Summary:     "Get workflow run labels",
		Description: "Returns the labels attached to a workflow run.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-workflow-step-runs",
		Method:      http.MethodGet,
		Path:        "/v1/workflow-runs/{workflowRunID}/steps",
		Summary:     "List workflow step runs",
		Description: "Returns all step runs for a specific workflow run.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
	}) (*struct{ Body []domain.WorkflowStepRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-workflow-run-graph",
		Method:      http.MethodGet,
		Path:        "/v1/workflow-runs/{workflowRunID}/graph",
		Summary:     "Get workflow run graph",
		Description: "Returns the execution graph for a workflow run with step statuses.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-workflow-run-explain",
		Method:      http.MethodGet,
		Path:        "/v1/workflow-runs/{workflowRunID}/explain",
		Summary:     "Explain workflow run execution",
		Description: "Returns a human-readable explanation of workflow run execution decisions.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-workflow-run-timeline",
		Method:      http.MethodGet,
		Path:        "/v1/workflow-runs/{workflowRunID}/timeline",
		Summary:     "Get workflow run timeline",
		Description: "Returns a chronological timeline of events for a workflow run.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
	}) (*struct{ Body domain.TimelineResponse }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "retry-workflow-run",
		Method:      http.MethodPost,
		Path:        "/v1/workflow-runs/{workflowRunID}/retry",
		Summary:     "Retry a workflow run",
		Description: "Retries a failed workflow run from the beginning.",
		Tags:        []string{"Workflow Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
	}) (*struct{ Body domain.WorkflowRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	// Workflow step operations within a workflow run.
	huma.Register(api, huma.Operation{
		OperationID: "approve-workflow-step",
		Method:      http.MethodPost,
		Path:        "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/approve",
		Summary:     "Approve a workflow step",
		Description: "Approves a workflow step that is waiting for manual approval.",
		Tags:        []string{"Workflow Steps"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
		StepRef       string `path:"stepRef" doc:"Step reference name" example:"validate-input"`
	}) (*struct{ Body domain.WorkflowStepRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "skip-workflow-step",
		Method:      http.MethodPost,
		Path:        "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/skip",
		Summary:     "Skip a workflow step",
		Description: "Skips a pending workflow step and continues execution.",
		Tags:        []string{"Workflow Steps"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
		StepRef       string `path:"stepRef" doc:"Step reference name" example:"validate-input"`
	}) (*struct{ Body domain.WorkflowStepRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "force-complete-workflow-step",
		Method:      http.MethodPost,
		Path:        "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/force-complete",
		Summary:     "Force-complete a workflow step",
		Description: "Forces a workflow step to complete regardless of its current state.",
		Tags:        []string{"Workflow Steps"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
		StepRef       string `path:"stepRef" doc:"Step reference name" example:"validate-input"`
	}) (*struct{ Body domain.WorkflowStepRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "retry-workflow-step",
		Method:      http.MethodPost,
		Path:        "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/retry",
		Summary:     "Retry a workflow step",
		Description: "Retries a failed workflow step.",
		Tags:        []string{"Workflow Steps"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
		StepRef       string `path:"stepRef" doc:"Step reference name" example:"validate-input"`
	}) (*struct{ Body domain.WorkflowStepRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "replay-workflow-subtree",
		Method:      http.MethodPost,
		Path:        "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/replay-subtree",
		Summary:     "Replay a workflow subtree",
		Description: "Replays a workflow step and all of its downstream dependent steps.",
		Tags:        []string{"Workflow Steps"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowRunID string `path:"workflowRunID" doc:"Workflow run ID" example:"wfrun_01HX9DVSW7"`
		StepRef       string `path:"stepRef" doc:"Step reference name" example:"validate-input"`
	}) (*struct{ Body domain.WorkflowRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerDeploymentOps registers deployment version management operations.

func (s *Server) registerDeploymentOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-deployment",
		Method:      http.MethodPost,
		Path:        "/v1/deployments",
		Summary:     "Create a deployment version",
		Description: "Creates a new deployment version for a workflow.",
		Tags:        []string{"Deployments"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body domain.DeploymentVersion }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-deployments",
		Method:      http.MethodGet,
		Path:        "/v1/deployments",
		Summary:     "List deployment versions",
		Description: "Returns a paginated list of deployment versions.",
		Tags:        []string{"Deployments"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.DeploymentVersion }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "finalize-deployment",
		Method:      http.MethodPost,
		Path:        "/v1/deployments/{deploymentID}/finalize",
		Summary:     "Finalize a deployment version",
		Description: "Finalizes a deployment version, locking its configuration.",
		Tags:        []string{"Deployments"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		DeploymentID string `path:"deploymentID" doc:"Deployment version ID" example:"dep_01HX9HMVR8"`
	}) (*struct{ Body domain.DeploymentVersion }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "promote-deployment",
		Method:      http.MethodPost,
		Path:        "/v1/deployments/{deploymentID}/promote",
		Summary:     "Promote a deployment version",
		Description: "Promotes a deployment version to active, routing traffic to it.",
		Tags:        []string{"Deployments"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		DeploymentID string `path:"deploymentID" doc:"Deployment version ID" example:"dep_01HX9HMVR8"`
		Body         any
	}) (*struct{ Body domain.DeploymentVersion }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "rollback-deployment",
		Method:      http.MethodPost,
		Path:        "/v1/deployments/{deploymentID}/rollback",
		Summary:     "Rollback a deployment version",
		Description: "Rolls back to the previous deployment version.",
		Tags:        []string{"Deployments"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		DeploymentID string `path:"deploymentID" doc:"Deployment version ID" example:"dep_01HX9HMVR8"`
	}) (*struct{ Body domain.DeploymentVersion }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerEventOps registers event trigger operations.

func (s *Server) registerEventOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-event-triggers",
		Method:      http.MethodGet,
		Path:        "/v1/events",
		Summary:     "List event triggers",
		Description: "Returns a paginated list of event triggers in the current project.",
		Tags:        []string{"Events"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.EventTrigger }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-event-trigger-stats",
		Method:      http.MethodGet,
		Path:        "/v1/events/stats",
		Summary:     "Get event trigger statistics",
		Description: "Returns aggregate statistics about event triggers.",
		Tags:        []string{"Events"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "purge-event-triggers",
		Method:      http.MethodPost,
		Path:        "/v1/events/purge",
		Summary:     "Purge event triggers",
		Description: "Purges completed or expired event triggers.",
		Tags:        []string{"Events"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "send-event-by-prefix",
		Method:      http.MethodPost,
		Path:        "/v1/events/prefix/{prefix}/send",
		Summary:     "Send event by prefix",
		Description: "Sends an event to all triggers matching the given key prefix.",
		Tags:        []string{"Events"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Prefix string `path:"prefix" doc:"Event key prefix" example:"order.completed"`
		Body   any
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-event-trigger",
		Method:      http.MethodGet,
		Path:        "/v1/events/{eventKey}",
		Summary:     "Get an event trigger",
		Description: "Returns details of a specific event trigger by its key.",
		Tags:        []string{"Events"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		EventKey string `path:"eventKey" doc:"Event key" example:"order.completed.12345"`
	}) (*struct{ Body domain.EventTrigger }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "cancel-event-trigger",
		Method:      http.MethodDelete,
		Path:        "/v1/events/{eventKey}",
		Summary:     "Cancel an event trigger",
		Description: "Cancels a waiting event trigger.",
		Tags:        []string{"Events"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		EventKey string `path:"eventKey" doc:"Event key" example:"order.completed.12345"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "send-event",
		Method:      http.MethodPost,
		Path:        "/v1/events/{eventKey}/send",
		Summary:     "Send an event",
		Description: "Sends a payload to an event trigger, resolving it.",
		Tags:        []string{"Events"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		EventKey string `path:"eventKey" doc:"Event key" example:"order.completed.12345"`
		Body     any
	}) (*struct{ Body domain.EventTrigger }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "dispatch-event",
		Method:      http.MethodPost,
		Path:        "/v1/events/dispatch",
		Summary:     "Dispatch an event",
		Description: "Dispatches an event that triggers matching subscriptions.",
		Tags:        []string{"Events"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "stream-event-trigger",
		Method:      http.MethodGet,
		Path:        "/v1/events/{eventKey}/stream",
		Summary:     "Stream event trigger updates",
		Description: "Opens an SSE stream for real-time updates on an event trigger.",
		Tags:        []string{"Events"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		EventKey string `path:"eventKey" doc:"Event key" example:"order.completed.12345"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerEventSourceOps registers event source management operations.

func (s *Server) registerEventSourceOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-event-sources",
		Method:      http.MethodGet,
		Path:        "/v1/event-sources",
		Summary:     "List event sources",
		Description: "Returns all event sources configured in the current project.",
		Tags:        []string{"Event Sources"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.EventSource }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-event-source",
		Method:      http.MethodPost,
		Path:        "/v1/event-sources",
		Summary:     "Create an event source",
		Description: "Creates a new event source for receiving external events.",
		Tags:        []string{"Event Sources"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body domain.EventSource }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-event-source",
		Method:      http.MethodGet,
		Path:        "/v1/event-sources/{sourceID}",
		Summary:     "Get an event source",
		Description: "Returns details of a specific event source.",
		Tags:        []string{"Event Sources"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		SourceID string `path:"sourceID" doc:"Event source ID" example:"src_01HX9JNWT4"`
	}) (*struct{ Body domain.EventSource }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-event-source",
		Method:      http.MethodPatch,
		Path:        "/v1/event-sources/{sourceID}",
		Summary:     "Update an event source",
		Description: "Updates an existing event source's configuration.",
		Tags:        []string{"Event Sources"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		SourceID string `path:"sourceID" doc:"Event source ID" example:"src_01HX9JNWT4"`
		Body     any
	}) (*struct{ Body domain.EventSource }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-event-source",
		Method:      http.MethodDelete,
		Path:        "/v1/event-sources/{sourceID}",
		Summary:     "Delete an event source",
		Description: "Permanently deletes an event source and its subscriptions.",
		Tags:        []string{"Event Sources"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		SourceID string `path:"sourceID" doc:"Event source ID" example:"src_01HX9JNWT4"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-event-source-subscriptions",
		Method:      http.MethodGet,
		Path:        "/v1/event-sources/{sourceID}/subscriptions",
		Summary:     "List event source subscriptions",
		Description: "Returns all subscriptions for a specific event source.",
		Tags:        []string{"Event Sources"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		SourceID string `path:"sourceID" doc:"Event source ID" example:"src_01HX9JNWT4"`
	}) (*struct{ Body []domain.EventSubscription }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "subscribe-to-event-source",
		Method:      http.MethodPost,
		Path:        "/v1/event-sources/{sourceID}/subscribe",
		Summary:     "Subscribe to an event source",
		Description: "Creates a subscription linking a job or workflow to an event source.",
		Tags:        []string{"Event Sources"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		SourceID string `path:"sourceID" doc:"Event source ID" example:"src_01HX9JNWT4"`
		Body     any
	}) (*struct{ Body domain.EventSubscription }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-event-subscription",
		Method:      http.MethodDelete,
		Path:        "/v1/event-sources/{sourceID}/subscriptions/{subID}",
		Summary:     "Delete an event subscription",
		Description: "Removes a subscription from an event source.",
		Tags:        []string{"Event Sources"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		SourceID string `path:"sourceID" doc:"Event source ID" example:"src_01HX9JNWT4"`
		SubID    string `path:"subID" doc:"Subscription ID" example:"sub_01HX9KPXV5"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerWebhookOps registers webhook management operations.

func (s *Server) registerWebhookOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "test-webhook",
		Method:      http.MethodPost,
		Path:        "/v1/webhooks/test",
		Summary:     "Test a webhook",
		Description: "Sends a test payload to a webhook URL to verify connectivity.",
		Tags:        []string{"Webhooks"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-webhook-deliveries",
		Method:      http.MethodGet,
		Path:        "/v1/webhooks/deliveries",
		Summary:     "List webhook deliveries",
		Description: "Returns a paginated list of webhook delivery attempts.",
		Tags:        []string{"Webhooks"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.WebhookDelivery }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-webhook-delivery",
		Method:      http.MethodGet,
		Path:        "/v1/webhooks/deliveries/{id}",
		Summary:     "Get a webhook delivery",
		Description: "Returns details of a specific webhook delivery attempt.",
		Tags:        []string{"Webhooks"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		ID string `path:"id" doc:"Webhook delivery ID" example:"del_01HX9LQXW5"`
	}) (*struct{ Body domain.WebhookDelivery }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "retry-webhook-delivery",
		Method:      http.MethodPost,
		Path:        "/v1/webhooks/deliveries/{id}/retry",
		Summary:     "Retry a webhook delivery",
		Description: "Retries a failed webhook delivery attempt.",
		Tags:        []string{"Webhooks"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		ID string `path:"id" doc:"Webhook delivery ID" example:"del_01HX9LQXW5"`
	}) (*struct{ Body domain.WebhookDelivery }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "replay-webhook-delivery",
		Method:      http.MethodPost,
		Path:        "/v1/webhooks/deliveries/{id}/replay",
		Summary:     "Replay a webhook delivery",
		Description: "Replays a webhook delivery with the original payload.",
		Tags:        []string{"Webhooks"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		ID string `path:"id" doc:"Webhook delivery ID" example:"del_01HX9LQXW5"`
	}) (*struct{ Body domain.WebhookDelivery }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-webhook-subscription",
		Method:      http.MethodPost,
		Path:        "/v1/webhooks/subscriptions",
		Summary:     "Create a webhook subscription",
		Description: "Creates a new webhook subscription to receive event notifications.",
		Tags:        []string{"Webhooks"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body domain.WebhookSubscription }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-webhook-subscriptions",
		Method:      http.MethodGet,
		Path:        "/v1/webhooks/subscriptions",
		Summary:     "List webhook subscriptions",
		Description: "Returns all webhook subscriptions in the current project.",
		Tags:        []string{"Webhooks"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.WebhookSubscription }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-webhook-subscription",
		Method:      http.MethodDelete,
		Path:        "/v1/webhooks/subscriptions/{id}",
		Summary:     "Delete a webhook subscription",
		Description: "Removes a webhook subscription, stopping further deliveries.",
		Tags:        []string{"Webhooks"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		ID string `path:"id" doc:"Webhook subscription ID" example:"wsub_01HX9LRYW6"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	// Legacy top-level webhook delivery routes.
	huma.Register(api, huma.Operation{
		OperationID: "list-webhook-deliveries-legacy",
		Method:      http.MethodGet,
		Path:        "/v1/webhook-deliveries",
		Summary:     "List webhook deliveries (legacy)",
		Description: "Returns a paginated list of webhook delivery attempts. Legacy endpoint, prefer /v1/webhooks/deliveries.",
		Tags:        []string{"Webhooks"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.WebhookDelivery }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "retry-webhook-delivery-legacy",
		Method:      http.MethodPost,
		Path:        "/v1/webhook-deliveries/{deliveryID}/retry",
		Summary:     "Retry a webhook delivery (legacy)",
		Description: "Retries a failed webhook delivery. Legacy endpoint, prefer /v1/webhooks/deliveries/{id}/retry.",
		Tags:        []string{"Webhooks"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		DeliveryID string `path:"deliveryID" doc:"Webhook delivery ID" example:"del_01HX9LQXW5"`
	}) (*struct{ Body domain.WebhookDelivery }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerAPIKeyOps registers API key management operations.

func (s *Server) registerAPIKeyOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-api-key",
		Method:      http.MethodPost,
		Path:        "/v1/api-keys",
		Summary:     "Create an API key",
		Description: "Creates a new API key for authenticating with the API.",
		Tags:        []string{"API Keys"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			Name   string   `json:"name" required:"true" doc:"Human-readable key name" example:"production-key"`
			Scopes []string `json:"scopes,omitempty" doc:"Permission scopes for this key" example:"jobs:read,runs:write"`
		}
	}) (*struct{ Body domain.APIKey }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-api-keys",
		Method:      http.MethodGet,
		Path:        "/v1/api-keys",
		Summary:     "List API keys",
		Description: "Returns all API keys for the current project.",
		Tags:        []string{"API Keys"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.APIKey }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "rotate-api-key",
		Method:      http.MethodPost,
		Path:        "/v1/api-keys/{keyID}/rotate",
		Summary:     "Rotate an API key",
		Description: "Rotates an API key, generating a new secret while keeping the same ID.",
		Tags:        []string{"API Keys"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		KeyID string `path:"keyID" doc:"API key ID" example:"key_01HX8GMNV6"`
	}) (*struct{ Body domain.APIKey }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "revoke-api-key",
		Method:      http.MethodDelete,
		Path:        "/v1/api-keys/{keyID}",
		Summary:     "Revoke an API key",
		Description: "Permanently revokes an API key, immediately invalidating it.",
		Tags:        []string{"API Keys"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		KeyID string `path:"keyID" doc:"API key ID" example:"key_01HX8GMNV6"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerCLIAuthOps registers CLI device authorization operations.

func (s *Server) registerCLIAuthOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "request-device-code",
		Method:      http.MethodPost,
		Path:        "/v1/cli/auth/device-code",
		Summary:     "Request a device authorization code",
		Description: "Initiates the device authorization flow by generating a device code and user code.",
		Tags:        []string{"CLI Auth"},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct{}) (*struct {
		Body struct {
			DeviceCode      string `json:"device_code" doc:"Device code for polling" example:"ABCD-1234-EFGH-5678"`
			UserCode        string `json:"user_code" doc:"Code to display to user" example:"STRAIT-A1B2"`
			VerificationURI string `json:"verification_uri" doc:"URL for user to visit" example:"https://app.strait.dev/device"`
			ExpiresIn       int    `json:"expires_in" doc:"Seconds until code expires" example:"900"`
			Interval        int    `json:"interval" doc:"Polling interval in seconds" example:"5"`
		}
	}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "poll-device-token",
		Method:      http.MethodPost,
		Path:        "/v1/cli/auth/token",
		Summary:     "Poll for device token",
		Description: "Polls for the authorization token after the user approves the device code.",
		Tags:        []string{"CLI Auth"},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			DeviceCode string `json:"device_code" required:"true" doc:"Device code from the initial request" example:"ABCD-1234-EFGH-5678"`
		}
	}) (*struct {
		Body struct {
			AccessToken string `json:"access_token" doc:"Bearer token for API access" example:"sk_live_01HX8GMNV6..."`
			TokenType   string `json:"token_type" doc:"Token type (bearer)" example:"bearer"`
		}
	}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "approve-device-code",
		Method:      http.MethodPost,
		Path:        "/v1/cli/device-codes/approve",
		Summary:     "Approve a device code",
		Description: "Approves a pending device code, authorizing the CLI session.",
		Tags:        []string{"CLI Auth"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			UserCode string `json:"user_code" required:"true" doc:"User code to approve" example:"STRAIT-A1B2"`
		}
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerSecretOps registers secret management operations.

func (s *Server) registerSecretOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-secret",
		Method:      http.MethodPost,
		Path:        "/v1/secrets",
		Summary:     "Create a secret",
		Description: "Creates a new encrypted secret for use in job payloads.",
		Tags:        []string{"Secrets"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			Name  string `json:"name" required:"true" doc:"Secret name" example:"DATABASE_URL"`
			Value string `json:"value" required:"true" doc:"Secret value" example:"postgres://user:pass@host:5432/db"`
		}
	}) (*struct{ Body domain.JobSecret }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-secrets",
		Method:      http.MethodGet,
		Path:        "/v1/secrets",
		Summary:     "List secrets",
		Description: "Returns all secrets in the current project. Values are redacted.",
		Tags:        []string{"Secrets"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.JobSecret }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-secret",
		Method:      http.MethodDelete,
		Path:        "/v1/secrets/{secretID}",
		Summary:     "Delete a secret",
		Description: "Permanently deletes a secret.",
		Tags:        []string{"Secrets"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		SecretID string `path:"secretID" doc:"Secret ID" example:"sec_01HX8HPQR7"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerLogDrainOps registers log drain management operations.

func (s *Server) registerLogDrainOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-log-drains",
		Method:      http.MethodGet,
		Path:        "/v1/log-drains",
		Summary:     "List log drains",
		Description: "Returns all log drains configured in the current project.",
		Tags:        []string{"Log Drains"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.LogDrain }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-log-drain",
		Method:      http.MethodPost,
		Path:        "/v1/log-drains",
		Summary:     "Create a log drain",
		Description: "Creates a new log drain to forward job execution logs to an external destination.",
		Tags:        []string{"Log Drains"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body domain.LogDrain }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-log-drain",
		Method:      http.MethodGet,
		Path:        "/v1/log-drains/{drainID}",
		Summary:     "Get a log drain",
		Description: "Returns details of a specific log drain.",
		Tags:        []string{"Log Drains"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		DrainID string `path:"drainID" doc:"Log drain ID" example:"drain_01HX9MSYW7"`
	}) (*struct{ Body domain.LogDrain }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-log-drain",
		Method:      http.MethodPatch,
		Path:        "/v1/log-drains/{drainID}",
		Summary:     "Update a log drain",
		Description: "Updates an existing log drain's configuration.",
		Tags:        []string{"Log Drains"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		DrainID string `path:"drainID" doc:"Log drain ID" example:"drain_01HX9MSYW7"`
		Body    any
	}) (*struct{ Body domain.LogDrain }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-log-drain",
		Method:      http.MethodDelete,
		Path:        "/v1/log-drains/{drainID}",
		Summary:     "Delete a log drain",
		Description: "Permanently deletes a log drain.",
		Tags:        []string{"Log Drains"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		DrainID string `path:"drainID" doc:"Log drain ID" example:"drain_01HX9MSYW7"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerNotificationOps registers notification channel and delivery operations.

func (s *Server) registerNotificationOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-notification-channel",
		Method:      http.MethodPost,
		Path:        "/v1/notification-channels",
		Summary:     "Create a notification channel",
		Description: "Creates a new notification channel for receiving alerts about job events.",
		Tags:        []string{"Notifications"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body domain.NotificationChannel }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-notification-channels",
		Method:      http.MethodGet,
		Path:        "/v1/notification-channels",
		Summary:     "List notification channels",
		Description: "Returns all notification channels in the current project.",
		Tags:        []string{"Notifications"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.NotificationChannel }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-notification-channel",
		Method:      http.MethodGet,
		Path:        "/v1/notification-channels/{channelID}",
		Summary:     "Get a notification channel",
		Description: "Returns details of a specific notification channel.",
		Tags:        []string{"Notifications"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		ChannelID string `path:"channelID" doc:"Notification channel ID" example:"chan_01HX9NTZY8"`
	}) (*struct{ Body domain.NotificationChannel }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-notification-channel",
		Method:      http.MethodPatch,
		Path:        "/v1/notification-channels/{channelID}",
		Summary:     "Update a notification channel",
		Description: "Updates an existing notification channel's configuration.",
		Tags:        []string{"Notifications"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		ChannelID string `path:"channelID" doc:"Notification channel ID" example:"chan_01HX9NTZY8"`
		Body      any
	}) (*struct{ Body domain.NotificationChannel }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-notification-channel",
		Method:      http.MethodDelete,
		Path:        "/v1/notification-channels/{channelID}",
		Summary:     "Delete a notification channel",
		Description: "Permanently deletes a notification channel.",
		Tags:        []string{"Notifications"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		ChannelID string `path:"channelID" doc:"Notification channel ID" example:"chan_01HX9NTZY8"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-notification-deliveries",
		Method:      http.MethodGet,
		Path:        "/v1/notification-deliveries",
		Summary:     "List notification deliveries",
		Description: "Returns a paginated list of notification delivery attempts.",
		Tags:        []string{"Notifications"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.NotificationDelivery }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerRBACOps registers role-based access control operations.

func (s *Server) registerRBACOps(api huma.API) {
	// Roles
	huma.Register(api, huma.Operation{
		OperationID: "create-role",
		Method:      http.MethodPost,
		Path:        "/v1/roles",
		Summary:     "Create a role",
		Description: "Creates a new custom role with the specified permissions.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body domain.ProjectRole }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-roles",
		Method:      http.MethodGet,
		Path:        "/v1/roles",
		Summary:     "List roles",
		Description: "Returns all roles defined in the current project.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.ProjectRole }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-role",
		Method:      http.MethodGet,
		Path:        "/v1/roles/{roleID}",
		Summary:     "Get a role",
		Description: "Returns details of a specific role including its permissions.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RoleID string `path:"roleID" doc:"Role ID" example:"role_01HX8KNXP3"`
	}) (*struct{ Body domain.ProjectRole }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-role",
		Method:      http.MethodPatch,
		Path:        "/v1/roles/{roleID}",
		Summary:     "Update a role",
		Description: "Updates an existing role's name or permissions.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RoleID string `path:"roleID" doc:"Role ID" example:"role_01HX8KNXP3"`
		Body   any
	}) (*struct{ Body domain.ProjectRole }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-role",
		Method:      http.MethodDelete,
		Path:        "/v1/roles/{roleID}",
		Summary:     "Delete a role",
		Description: "Permanently deletes a custom role. Members with this role lose its permissions.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RoleID string `path:"roleID" doc:"Role ID" example:"role_01HX8KNXP3"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	// Members
	huma.Register(api, huma.Operation{
		OperationID: "assign-member",
		Method:      http.MethodPost,
		Path:        "/v1/members",
		Summary:     "Assign a member to a role",
		Description: "Assigns a user to a role in the current project.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			UserID string `json:"user_id" required:"true" doc:"User ID to assign" example:"user_01HX8JKWM9"`
			RoleID string `json:"role_id" required:"true" doc:"Role ID to assign" example:"role_01HX8KNXP3"`
		}
	}) (*struct{ Body domain.ProjectMemberRole }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "bulk-assign-members",
		Method:      http.MethodPost,
		Path:        "/v1/members/bulk",
		Summary:     "Bulk assign members",
		Description: "Assigns multiple users to roles in a single operation.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			Assignments []struct {
				UserID string `json:"user_id" required:"true" doc:"User ID" example:"user_01HX8JKWM9"`
				RoleID string `json:"role_id" required:"true" doc:"Role ID" example:"role_01HX8KNXP3"`
			} `json:"assignments" required:"true" doc:"List of role assignments"`
		}
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-members",
		Method:      http.MethodGet,
		Path:        "/v1/members",
		Summary:     "List members",
		Description: "Returns all members and their roles in the current project.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.ProjectMemberRole }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "remove-member",
		Method:      http.MethodDelete,
		Path:        "/v1/members/{userID}",
		Summary:     "Remove a member",
		Description: "Removes a user's role assignment from the current project.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		UserID string `path:"userID" doc:"User ID to remove" example:"user_01HX8JKWM9"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	// Seed roles
	huma.Register(api, huma.Operation{
		OperationID: "seed-system-roles",
		Method:      http.MethodPost,
		Path:        "/v1/seed-roles",
		Summary:     "Seed system roles",
		Description: "Creates the default system roles (admin, editor, viewer) if they do not exist.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	// Resource policies
	huma.Register(api, huma.Operation{
		OperationID: "create-resource-policy",
		Method:      http.MethodPost,
		Path:        "/v1/resource-policies",
		Summary:     "Create a resource policy",
		Description: "Creates a policy restricting access to specific resources by role.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body domain.ResourcePolicy }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-resource-policies",
		Method:      http.MethodGet,
		Path:        "/v1/resource-policies",
		Summary:     "List resource policies",
		Description: "Returns all resource policies in the current project.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.ResourcePolicy }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-resource-policy",
		Method:      http.MethodDelete,
		Path:        "/v1/resource-policies/{policyID}",
		Summary:     "Delete a resource policy",
		Description: "Permanently deletes a resource policy.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		PolicyID string `path:"policyID" doc:"Resource policy ID" example:"pol_01HX9PVZW9"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	// Tag policies
	huma.Register(api, huma.Operation{
		OperationID: "create-tag-policy",
		Method:      http.MethodPost,
		Path:        "/v1/tag-policies",
		Summary:     "Create a tag policy",
		Description: "Creates a policy restricting access based on resource tags.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body domain.TagPolicy }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-tag-policies",
		Method:      http.MethodGet,
		Path:        "/v1/tag-policies",
		Summary:     "List tag policies",
		Description: "Returns all tag policies in the current project.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.TagPolicy }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-tag-policy",
		Method:      http.MethodDelete,
		Path:        "/v1/tag-policies/{policyID}",
		Summary:     "Delete a tag policy",
		Description: "Permanently deletes a tag policy.",
		Tags:        []string{"RBAC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		PolicyID string `path:"policyID" doc:"Tag policy ID" example:"tpol_01HX9QWAX1"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerAuditOps registers audit event operations.

func (s *Server) registerAuditOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-audit-events",
		Method:      http.MethodGet,
		Path:        "/v1/audit-events",
		Summary:     "List audit events",
		Description: "Returns a paginated list of audit events for the current project.",
		Tags:        []string{"Audit"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.AuditEvent }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "export-audit-events",
		Method:      http.MethodGet,
		Path:        "/v1/audit-events/export",
		Summary:     "Export audit events",
		Description: "Exports audit events as CSV or JSON for compliance and reporting.",
		Tags:        []string{"Audit"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Format string `query:"format" doc:"Export format" enum:"csv,json" example:"json"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerBillingOps registers billing and usage operations.

func (s *Server) registerBillingOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-current-usage",
		Method:      http.MethodGet,
		Path:        "/v1/usage/current",
		Summary:     "Get current usage",
		Description: "Returns the current billing period's usage metrics.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-usage-history",
		Method:      http.MethodGet,
		Path:        "/v1/usage/history",
		Summary:     "Get usage history",
		Description: "Returns historical usage data across billing periods.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Periods int `query:"periods" doc:"Number of historical periods to return" example:"6"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-usage-forecast",
		Method:      http.MethodGet,
		Path:        "/v1/usage/forecast",
		Summary:     "Get usage forecast",
		Description: "Returns projected usage for the current billing period.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-project-costs",
		Method:      http.MethodGet,
		Path:        "/v1/usage/projects",
		Summary:     "Get project costs",
		Description: "Returns cost breakdown by project for the current billing period.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-anomaly-alerts",
		Method:      http.MethodGet,
		Path:        "/v1/usage/anomalies",
		Summary:     "Get anomaly alerts",
		Description: "Returns usage anomaly alerts based on configured thresholds.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "export-usage",
		Method:      http.MethodGet,
		Path:        "/v1/usage/export",
		Summary:     "Export usage data",
		Description: "Exports usage data as CSV or JSON for external analysis.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Format string `query:"format" doc:"Export format" enum:"csv,json" example:"json"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-spending-limit",
		Method:      http.MethodGet,
		Path:        "/v1/spending-limit",
		Summary:     "Get spending limit",
		Description: "Returns the current spending limit configuration.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-spending-limit",
		Method:      http.MethodPut,
		Path:        "/v1/spending-limit",
		Summary:     "Update spending limit",
		Description: "Sets or updates the spending limit for the current project.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-cost-estimate",
		Method:      http.MethodGet,
		Path:        "/v1/cost-estimate",
		Summary:     "Get cost estimate",
		Description: "Returns a cost estimate based on current usage patterns.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-downgrade-preview",
		Method:      http.MethodGet,
		Path:        "/v1/downgrade-preview",
		Summary:     "Get downgrade preview",
		Description: "Returns a preview of the impact of downgrading to a lower plan tier.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-project-budget",
		Method:      http.MethodGet,
		Path:        "/v1/project-budget",
		Summary:     "Get project budget",
		Description: "Returns the budget configuration for the current project.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-project-budget",
		Method:      http.MethodPut,
		Path:        "/v1/project-budget",
		Summary:     "Update project budget",
		Description: "Sets or updates the budget for the current project.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-anomaly-config",
		Method:      http.MethodGet,
		Path:        "/v1/anomaly-config",
		Summary:     "Get anomaly detection config",
		Description: "Returns the anomaly detection configuration for the current project.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-anomaly-config",
		Method:      http.MethodPut,
		Path:        "/v1/anomaly-config",
		Summary:     "Update anomaly detection config",
		Description: "Sets or updates the anomaly detection thresholds for the current project.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "check-org-limit",
		Method:      http.MethodGet,
		Path:        "/v1/billing/check-org-limit",
		Summary:     "Check organization limit",
		Description: "Checks whether the organization has reached its plan limits.",
		Tags:        []string{"Billing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerReferralOps registers referral operations.

func (s *Server) registerReferralOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-referral-code",
		Method:      http.MethodPost,
		Path:        "/v1/referrals",
		Summary:     "Create a referral code",
		Description: "Generates a new referral code for sharing with other users.",
		Tags:        []string{"Referrals"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "activate-referral",
		Method:      http.MethodPost,
		Path:        "/v1/referrals/activate",
		Summary:     "Activate a referral code",
		Description: "Activates a referral code to receive the referral benefit.",
		Tags:        []string{"Referrals"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			Code string `json:"code" required:"true" doc:"Referral code to activate" example:"STRAIT-REF-2024"`
		}
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-referrals",
		Method:      http.MethodGet,
		Path:        "/v1/referrals",
		Summary:     "List referrals",
		Description: "Returns all referral codes and their activation status.",
		Tags:        []string{"Referrals"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerAnalyticsOps registers analytics operations.

func (s *Server) registerAnalyticsOps(api huma.API) {
	// Community analytics (Postgres-backed)
	huma.Register(api, huma.Operation{
		OperationID: "get-performance-analytics",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/performance",
		Summary:     "Get performance analytics",
		Description: "Returns job execution performance metrics including p50/p95/p99 latencies.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" enum:"1h,6h,24h,7d,30d" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-cost-analytics",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/costs",
		Summary:     "Get cost analytics",
		Description: "Returns cost analytics for the current billing period.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-cost-trends",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/costs/trends",
		Summary:     "Get cost trends",
		Description: "Returns cost trend data over time for identifying spending patterns.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-top-costs",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/costs/top",
		Summary:     "Get top cost contributors",
		Description: "Returns the jobs contributing most to overall costs.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
		Limit  int    `query:"limit" doc:"Max results" example:"10"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-compute-cost-analytics",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/compute",
		Summary:     "Get compute cost analytics",
		Description: "Returns compute resource utilization and cost breakdown.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-approval-stats",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/approvals",
		Summary:     "Get approval statistics",
		Description: "Returns statistics about workflow approval steps including wait times.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-cost-insights",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/cost-insights",
		Summary:     "Get cost insights",
		Description: "Returns actionable insights for reducing costs based on usage patterns.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	// Cloud-only analytics (ClickHouse-backed)
	huma.Register(api, huma.Operation{
		OperationID: "get-run-timeline",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/runs/timeline",
		Summary:     "Get run timeline",
		Description: "Returns time-series data of run executions bucketed by interval.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period   string `query:"period" doc:"Time period"`
		Interval string `query:"interval" doc:"Bucket interval" example:"1h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-run-duration-distribution",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/runs/duration-distribution",
		Summary:     "Get run duration distribution",
		Description: "Returns a histogram of run durations across percentile buckets.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-run-failure-reasons",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/runs/failure-reasons",
		Summary:     "Get run failure reasons",
		Description: "Returns the most common failure reasons across runs.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
		Limit  int    `query:"limit" doc:"Max results" example:"10"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-run-summary",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/runs/summary",
		Summary:     "Get run summary",
		Description: "Returns aggregate summary statistics for runs in the specified period.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-runs-by-trigger",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/runs/by-trigger",
		Summary:     "Get runs by trigger type",
		Description: "Returns run counts grouped by trigger type (cron, API, event, webhook).",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-job-comparison",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/jobs/comparison",
		Summary:     "Get job comparison",
		Description: "Returns side-by-side performance comparison across jobs.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-job-reliability",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/jobs/reliability",
		Summary:     "Get job reliability",
		Description: "Returns reliability metrics (success rate, MTTR, MTBF) for jobs.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-runs-by-version",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/jobs/by-version",
		Summary:     "Get runs by job version",
		Description: "Returns run metrics grouped by job version to track version performance.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-job-cost-ranking",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/jobs/cost-ranking",
		Summary:     "Get job cost ranking",
		Description: "Returns jobs ranked by total cost in the specified period.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
		Limit  int    `query:"limit" doc:"Max results" example:"10"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-top-failing-jobs",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/jobs/top-failing",
		Summary:     "Get top failing jobs",
		Description: "Returns jobs with the highest failure rates in the specified period.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
		Limit  int    `query:"limit" doc:"Max results" example:"10"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-job-history",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/jobs/{jobID}/history",
		Summary:     "Get job execution history",
		Description: "Returns historical execution data for a specific job.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		JobID  string `path:"jobID" doc:"Job ID"`
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-tag-summary",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/tags/summary",
		Summary:     "Get tag summary",
		Description: "Returns aggregate metrics grouped by tag for resource categorization.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-top-failing-tags",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/tags/top-failing",
		Summary:     "Get top failing tags",
		Description: "Returns tags associated with the highest failure rates.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
		Limit  int    `query:"limit" doc:"Max results" example:"10"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-tag-cost",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/tags/cost",
		Summary:     "Get tag cost breakdown",
		Description: "Returns cost data grouped by tag for cost allocation.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-workflow-completion-rates",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/workflows/completion-rates",
		Summary:     "Get workflow completion rates",
		Description: "Returns completion and success rates for workflows.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-workflow-analytics-summary",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/workflows/summary",
		Summary:     "Get workflow analytics summary",
		Description: "Returns aggregate analytics across all workflows.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-workflow-step-durations",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/workflows/{workflowID}/step-durations",
		Summary:     "Get workflow step durations",
		Description: "Returns average and percentile duration metrics for each step in a workflow.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		WorkflowID string `path:"workflowID" doc:"Workflow ID" example:"wf_01HX9CTRMK"`
		Period     string `query:"period" doc:"Time period"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-webhook-delivery-stats",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/webhooks/delivery-stats",
		Summary:     "Get webhook delivery statistics",
		Description: "Returns delivery success rates and latency metrics for webhooks.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-webhook-endpoint-health",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/webhooks/endpoint-health",
		Summary:     "Get webhook endpoint health",
		Description: "Returns health metrics for webhook endpoints including error rates.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-top-failing-webhooks",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/webhooks/top-failing",
		Summary:     "Get top failing webhooks",
		Description: "Returns webhook endpoints with the highest failure rates.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
		Limit  int    `query:"limit" doc:"Max results" example:"10"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-event-volume",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/events/volume",
		Summary:     "Get event volume",
		Description: "Returns event volume time-series data.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period   string `query:"period" doc:"Time period"`
		Interval string `query:"interval" doc:"Bucket interval" example:"1h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-event-latency",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/events/latency",
		Summary:     "Get event latency",
		Description: "Returns latency metrics for event processing.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-cost-forecast",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/costs/forecast",
		Summary:     "Get cost forecast",
		Description: "Returns projected costs based on historical usage trends.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-cost-by-trigger",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/costs/by-trigger",
		Summary:     "Get cost by trigger type",
		Description: "Returns cost breakdown grouped by trigger type.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-cost-by-machine",
		Method:      http.MethodGet,
		Path:        "/v1/analytics/costs/by-machine",
		Summary:     "Get cost by machine type",
		Description: "Returns cost breakdown grouped by machine/compute type.",
		Tags:        []string{"Analytics"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Period string `query:"period" doc:"Time period" example:"24h"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerSDKOps registers SDK callback operations used by job runners.

func (s *Server) registerSDKOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "sdk-get-payload",
		Method:      http.MethodGet,
		Path:        "/sdk/v1/runs/{runID}/payload",
		Summary:     "Get run payload",
		Description: "Returns the payload for a run so the SDK can begin execution.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-log",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/log",
		Summary:     "Send a log entry",
		Description: "Sends a structured log entry from the running job to Strait.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-progress",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/progress",
		Summary:     "Report execution progress",
		Description: "Reports execution progress as a percentage for monitoring.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  struct {
			Progress float64 `json:"progress" minimum:"0" maximum:"100" doc:"Progress percentage" example:"75.5"`
			Message  string  `json:"message,omitempty" doc:"Progress description" example:"Processing batch 3 of 4"`
		}
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-annotate",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/annotate",
		Summary:     "Annotate a run",
		Description: "Attaches metadata annotations to a run for search and filtering.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-heartbeat",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/heartbeat",
		Summary:     "Send a heartbeat",
		Description: "Sends a heartbeat to indicate the run is still actively executing.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-checkpoint",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/checkpoint",
		Summary:     "Save a checkpoint",
		Description: "Saves a checkpoint so the run can resume from this point on retry.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{ Body domain.RunCheckpoint }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-usage",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/usage",
		Summary:     "Report resource usage",
		Description: "Reports resource usage (tokens, compute time, etc.) for billing.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{ Body domain.RunUsage }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-tool-call",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/tool-call",
		Summary:     "Record a tool call",
		Description: "Records an LLM tool call for observability and debugging.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{ Body domain.RunToolCall }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-output",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/output",
		Summary:     "Record an output",
		Description: "Records a structured output produced by the run.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{ Body domain.RunOutput }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-complete",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/complete",
		Summary:     "Mark run as complete",
		Description: "Marks the run as successfully completed with optional result data.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-fail",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/fail",
		Summary:     "Mark run as failed",
		Description: "Marks the run as failed with an error message and optional details.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  struct {
			Error   string `json:"error" required:"true" doc:"Error message" example:"connection timeout after 30s"`
			Details any    `json:"details,omitempty" doc:"Additional error details as JSON"`
		}
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-spawn",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/spawn",
		Summary:     "Spawn a child run",
		Description: "Spawns a child run from within the current run for fan-out patterns.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{ Body domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-continue",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/continue",
		Summary:     "Continue execution",
		Description: "Signals that the run should continue with a new execution step.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-wait-for-event",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/wait-for-event",
		Summary:     "Wait for an external event",
		Description: "Pauses the run until an external event with the specified key is received.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  struct {
			EventKey    string `json:"event_key" required:"true" doc:"Event key to wait for" example:"payment.confirmed.12345"`
			TimeoutSecs int    `json:"timeout_secs,omitempty" doc:"Timeout in seconds" example:"3600"`
		}
	}) (*struct{ Body domain.EventTrigger }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-set-state",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/state",
		Summary:     "Set run state",
		Description: "Sets a key-value pair in the run's state store.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{ Body domain.RunState }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-list-state",
		Method:      http.MethodGet,
		Path:        "/sdk/v1/runs/{runID}/state",
		Summary:     "List run state",
		Description: "Returns all key-value pairs in the run's state store.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body []domain.RunState }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-get-state",
		Method:      http.MethodGet,
		Path:        "/sdk/v1/runs/{runID}/state/{key}",
		Summary:     "Get a state value",
		Description: "Returns a specific value from the run's state store by key.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Key   string `path:"key" doc:"State key" example:"last_cursor"`
	}) (*struct{ Body domain.RunState }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-delete-state",
		Method:      http.MethodDelete,
		Path:        "/sdk/v1/runs/{runID}/state/{key}",
		Summary:     "Delete a state value",
		Description: "Removes a key-value pair from the run's state store.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Key   string `path:"key" doc:"State key" example:"last_cursor"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-stream-chunk",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/stream",
		Summary:     "Send a stream chunk",
		Description: "Sends a streaming chunk for real-time output from LLM-powered runs.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-resources",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/resources",
		Summary:     "Report resource utilization",
		Description: "Reports CPU, memory, and other resource utilization during execution.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-resource-snapshot",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/resource-snapshot",
		Summary:     "Save a resource snapshot",
		Description: "Saves a point-in-time snapshot of resource utilization.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{ Body domain.RunResourceSnapshot }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-iteration",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/iteration",
		Summary:     "Record an iteration",
		Description: "Records an iteration in a loop-based execution pattern.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  any
	}) (*struct{ Body domain.RunIteration }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	// Memory operations
	huma.Register(api, huma.Operation{
		OperationID: "sdk-set-memory",
		Method:      http.MethodPost,
		Path:        "/sdk/v1/runs/{runID}/memory/{key}",
		Summary:     "Set a memory value",
		Description: "Stores a value in persistent memory that survives across run attempts.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Key   string `path:"key" doc:"Memory key" example:"processed_count"`
		Body  any
	}) (*struct{ Body domain.JobMemory }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-get-memory",
		Method:      http.MethodGet,
		Path:        "/sdk/v1/runs/{runID}/memory/{key}",
		Summary:     "Get a memory value",
		Description: "Retrieves a value from persistent memory by key.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Key   string `path:"key" doc:"Memory key" example:"processed_count"`
	}) (*struct{ Body domain.JobMemory }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-list-memory",
		Method:      http.MethodGet,
		Path:        "/sdk/v1/runs/{runID}/memory",
		Summary:     "List memory entries",
		Description: "Returns all key-value pairs in persistent memory for the run's job.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body []domain.JobMemory }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "sdk-delete-memory",
		Method:      http.MethodDelete,
		Path:        "/sdk/v1/runs/{runID}/memory/{key}",
		Summary:     "Delete a memory value",
		Description: "Removes a key-value pair from persistent memory.",
		Tags:        []string{"SDK"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Key   string `path:"key" doc:"Memory key" example:"processed_count"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerOrgQueryOps registers organization-scoped query operations.

func (s *Server) registerOrgQueryOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-org-runs",
		Method:      http.MethodGet,
		Path:        "/v1/organizations/{orgID}/runs",
		Summary:     "List runs across organization",
		Description: "Returns runs across all projects in an organization.",
		Tags:        []string{"Organizations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		OrgID  string `path:"orgID" doc:"Organization ID" example:"org_01HX7YMWQ3"`
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-org-jobs",
		Method:      http.MethodGet,
		Path:        "/v1/organizations/{orgID}/jobs",
		Summary:     "List jobs across organization",
		Description: "Returns jobs across all projects in an organization.",
		Tags:        []string{"Organizations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		OrgID  string `path:"orgID" doc:"Organization ID" example:"org_01HX7YMWQ3"`
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.Job }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerBatchOperationOps registers batch operation tracking operations.

func (s *Server) registerBatchOperationOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-batch-operations",
		Method:      http.MethodGet,
		Path:        "/v1/batch-operations",
		Summary:     "List batch operations",
		Description: "Returns a paginated list of batch operations and their progress.",
		Tags:        []string{"Batch Operations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.BatchOperation }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-batch-operation",
		Method:      http.MethodGet,
		Path:        "/v1/batch-operations/{batchID}",
		Summary:     "Get a batch operation",
		Description: "Returns details and progress of a specific batch operation.",
		Tags:        []string{"Batch Operations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		BatchID string `path:"batchID" doc:"Batch operation ID" example:"batch_01HX9MQYW6"`
	}) (*struct{ Body domain.BatchOperation }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerJobGroupOps registers job group management operations.

func (s *Server) registerJobGroupOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-job-group",
		Method:      http.MethodPost,
		Path:        "/v1/job-groups",
		Summary:     "Create a job group",
		Description: "Creates a new group for organizing related jobs.",
		Tags:        []string{"Job Groups"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body domain.JobGroup }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-job-groups",
		Method:      http.MethodGet,
		Path:        "/v1/job-groups",
		Summary:     "List job groups",
		Description: "Returns all job groups in the current project.",
		Tags:        []string{"Job Groups"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.JobGroup }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-job-group",
		Method:      http.MethodGet,
		Path:        "/v1/job-groups/{groupID}",
		Summary:     "Get a job group",
		Description: "Returns details of a specific job group.",
		Tags:        []string{"Job Groups"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		GroupID string `path:"groupID" doc:"Job group ID" example:"grp_01HX9NRZX7"`
	}) (*struct{ Body domain.JobGroup }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-job-group",
		Method:      http.MethodPatch,
		Path:        "/v1/job-groups/{groupID}",
		Summary:     "Update a job group",
		Description: "Updates an existing job group's name or configuration.",
		Tags:        []string{"Job Groups"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		GroupID string `path:"groupID" doc:"Job group ID" example:"grp_01HX9NRZX7"`
		Body    any
	}) (*struct{ Body domain.JobGroup }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-job-group",
		Method:      http.MethodDelete,
		Path:        "/v1/job-groups/{groupID}",
		Summary:     "Delete a job group",
		Description: "Permanently deletes a job group. Jobs in the group are not deleted.",
		Tags:        []string{"Job Groups"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		GroupID string `path:"groupID" doc:"Job group ID" example:"grp_01HX9NRZX7"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-jobs-by-group",
		Method:      http.MethodGet,
		Path:        "/v1/job-groups/{groupID}/jobs",
		Summary:     "List jobs in a group",
		Description: "Returns all jobs belonging to a specific job group.",
		Tags:        []string{"Job Groups"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		GroupID string `path:"groupID" doc:"Job group ID" example:"grp_01HX9NRZX7"`
	}) (*struct{ Body []domain.Job }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "pause-all-jobs-by-group",
		Method:      http.MethodPost,
		Path:        "/v1/job-groups/{groupID}/pause-all",
		Summary:     "Pause all jobs in a group",
		Description: "Pauses all jobs belonging to a specific job group.",
		Tags:        []string{"Job Groups"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		GroupID string `path:"groupID" doc:"Job group ID" example:"grp_01HX9NRZX7"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "resume-all-jobs-by-group",
		Method:      http.MethodPost,
		Path:        "/v1/job-groups/{groupID}/resume-all",
		Summary:     "Resume all jobs in a group",
		Description: "Resumes all paused jobs belonging to a specific job group.",
		Tags:        []string{"Job Groups"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		GroupID string `path:"groupID" doc:"Job group ID" example:"grp_01HX9NRZX7"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-job-group-stats",
		Method:      http.MethodGet,
		Path:        "/v1/job-groups/{groupID}/stats",
		Summary:     "Get job group statistics",
		Description: "Returns aggregate statistics for all jobs in a group.",
		Tags:        []string{"Job Groups"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		GroupID string `path:"groupID" doc:"Job group ID" example:"grp_01HX9NRZX7"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerEnvironmentOps registers environment management operations.

func (s *Server) registerEnvironmentOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-environment",
		Method:      http.MethodPost,
		Path:        "/v1/environments",
		Summary:     "Create an environment",
		Description: "Creates a new environment for isolating job configurations.",
		Tags:        []string{"Environments"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body any
	}) (*struct{ Body domain.Environment }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-environments",
		Method:      http.MethodGet,
		Path:        "/v1/environments",
		Summary:     "List environments",
		Description: "Returns all environments in the current project.",
		Tags:        []string{"Environments"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []domain.Environment }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-environment",
		Method:      http.MethodGet,
		Path:        "/v1/environments/{envID}",
		Summary:     "Get an environment",
		Description: "Returns details of a specific environment.",
		Tags:        []string{"Environments"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		EnvID string `path:"envID" doc:"Environment ID" example:"env_01HX9PSTY8"`
	}) (*struct{ Body domain.Environment }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-environment",
		Method:      http.MethodPatch,
		Path:        "/v1/environments/{envID}",
		Summary:     "Update an environment",
		Description: "Updates an existing environment's configuration.",
		Tags:        []string{"Environments"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		EnvID string `path:"envID" doc:"Environment ID" example:"env_01HX9PSTY8"`
		Body  any
	}) (*struct{ Body domain.Environment }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-environment",
		Method:      http.MethodDelete,
		Path:        "/v1/environments/{envID}",
		Summary:     "Delete an environment",
		Description: "Permanently deletes an environment.",
		Tags:        []string{"Environments"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		EnvID string `path:"envID" doc:"Environment ID" example:"env_01HX9PSTY8"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-resolved-variables",
		Method:      http.MethodGet,
		Path:        "/v1/environments/{envID}/variables",
		Summary:     "Get resolved variables",
		Description: "Returns the resolved environment variables with inheritance applied.",
		Tags:        []string{"Environments"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		EnvID string `path:"envID" doc:"Environment ID" example:"env_01HX9PSTY8"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerJobExtrasOps registers additional job-related operations.

func (s *Server) registerJobExtrasOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "batch-create-jobs",
		Method:      http.MethodPost,
		Path:        "/v1/jobs/batch",
		Summary:     "Batch create jobs",
		Description: "Creates multiple jobs in a single request.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			Jobs []CreateJobBody `json:"jobs" required:"true" doc:"List of jobs to create"`
		}
	}) (*struct{ Body []domain.Job }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "batch-enable-jobs",
		Method:      http.MethodPost,
		Path:        "/v1/jobs/batch-enable",
		Summary:     "Batch enable jobs",
		Description: "Enables multiple jobs in a single request.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			JobIDs []string `json:"job_ids" required:"true" doc:"List of job IDs to enable"`
		}
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "batch-disable-jobs",
		Method:      http.MethodPost,
		Path:        "/v1/jobs/batch-disable",
		Summary:     "Batch disable jobs",
		Description: "Disables multiple jobs in a single request.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			JobIDs []string `json:"job_ids" required:"true" doc:"List of job IDs to disable"`
		}
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "bulk-trigger-job",
		Method:      http.MethodPost,
		Path:        "/v1/jobs/{jobID}/trigger/bulk",
		Summary:     "Bulk trigger a job",
		Description: "Triggers multiple executions of a job with different payloads.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID" example:"job_01HX7YJKM3"`
		Body  any
	}) (*struct{ Body []domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-job-dependency",
		Method:      http.MethodPost,
		Path:        "/v1/jobs/{jobID}/dependencies",
		Summary:     "Create a job dependency",
		Description: "Creates a dependency relationship between two jobs.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 409, 429, 500},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID" example:"job_01HX7YJKM3"`
		Body  any
	}) (*struct{ Body domain.JobDependency }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-job-dependencies",
		Method:      http.MethodGet,
		Path:        "/v1/jobs/{jobID}/dependencies",
		Summary:     "List job dependencies",
		Description: "Returns all dependencies for a specific job.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID" example:"job_01HX7YJKM3"`
	}) (*struct{ Body []domain.JobDependency }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-job-dependency",
		Method:      http.MethodDelete,
		Path:        "/v1/jobs/{jobID}/dependencies/{depID}",
		Summary:     "Delete a job dependency",
		Description: "Removes a dependency relationship between two jobs.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID" example:"job_01HX7YJKM3"`
		DepID string `path:"depID" doc:"Dependency ID" example:"dep_01HX9QTVZ9"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-job-versions",
		Method:      http.MethodGet,
		Path:        "/v1/jobs/{jobID}/versions",
		Summary:     "List job versions",
		Description: "Returns all versions of a job definition showing configuration history.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID" example:"job_01HX7YJKM3"`
	}) (*struct{ Body []domain.JobVersion }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-job-version",
		Method:      http.MethodGet,
		Path:        "/v1/jobs/{jobID}/versions/{versionID}",
		Summary:     "Get a job version",
		Description: "Returns details of a specific job version.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		JobID     string `path:"jobID" doc:"Job ID"`
		VersionID string `path:"versionID" doc:"Version ID" example:"ver_01HX9FGTP2"`
	}) (*struct{ Body domain.JobVersion }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "clone-job",
		Method:      http.MethodPost,
		Path:        "/v1/jobs/{jobID}/clone",
		Summary:     "Clone a job",
		Description: "Creates a copy of an existing job with a new name.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID" example:"job_01HX7YJKM3"`
		Body  any
	}) (*struct{ Body domain.Job }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-job-health",
		Method:      http.MethodGet,
		Path:        "/v1/jobs/{jobID}/health",
		Summary:     "Get job health",
		Description: "Returns health metrics for a job including success rate and latency.",
		Tags:        []string{"Jobs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		JobID string `path:"jobID" doc:"Job ID" example:"job_01HX7YJKM3"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerRunExtrasOps registers additional run operations beyond basic CRUD.

func (s *Server) registerRunExtrasOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-dead-letter-runs",
		Method:      http.MethodGet,
		Path:        "/v1/runs/dlq",
		Summary:     "List dead-letter queue runs",
		Description: "Returns runs that have exhausted all retry attempts.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Max results" example:"20"`
		Cursor string `query:"cursor" doc:"Pagination cursor" example:"2024-01-15T09:00:00Z"`
	}) (*struct{ Body []domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "bulk-replay-dead-letter-runs",
		Method:      http.MethodPost,
		Path:        "/v1/runs/bulk-dlq-replay",
		Summary:     "Bulk replay dead-letter runs",
		Description: "Replays multiple runs from the dead-letter queue.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			RunIDs []string `json:"run_ids" required:"true" doc:"List of run IDs to replay"`
		}
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "bulk-cancel-runs",
		Method:      http.MethodPost,
		Path:        "/v1/runs/bulk-cancel",
		Summary:     "Bulk cancel runs",
		Description: "Cancels multiple runs matching the provided IDs.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			RunIDs []string `json:"run_ids" required:"true" doc:"List of run IDs to cancel"`
		}
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "bulk-cancel-all-runs",
		Method:      http.MethodPost,
		Path:        "/v1/runs/bulk-cancel-all",
		Summary:     "Bulk cancel all active runs",
		Description: "Cancels all active runs in the current project.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "bulk-replay-runs",
		Method:      http.MethodPost,
		Path:        "/v1/runs/bulk-replay",
		Summary:     "Bulk replay runs",
		Description: "Replays multiple runs matching the provided IDs.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		Body struct {
			RunIDs []string `json:"run_ids" required:"true" doc:"List of run IDs to replay"`
		}
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "replay-dead-letter-run",
		Method:      http.MethodPost,
		Path:        "/v1/runs/{runID}/dlq-replay",
		Summary:     "Replay a dead-letter run",
		Description: "Replays a single run from the dead-letter queue.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-child-runs",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/children",
		Summary:     "List child runs",
		Description: "Returns all child runs spawned by the specified run.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body []domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-run-events",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/events",
		Summary:     "List run events",
		Description: "Returns the event log for a specific run.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body []domain.RunEvent }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-run-checkpoints",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/checkpoints",
		Summary:     "List run checkpoints",
		Description: "Returns all checkpoints saved during a run's execution.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body []domain.RunCheckpoint }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-run-usage",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/usage",
		Summary:     "List run usage",
		Description: "Returns resource usage records for a specific run.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body []domain.RunUsage }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-run-tool-calls",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/tool-calls",
		Summary:     "List run tool calls",
		Description: "Returns all tool calls made during a run's execution.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body []domain.RunToolCall }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-run-outputs",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/outputs",
		Summary:     "List run outputs",
		Description: "Returns all structured outputs produced by a run.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body []domain.RunOutput }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-debug-bundle",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/debug-bundle",
		Summary:     "Get debug bundle",
		Description: "Returns a comprehensive debug bundle with all run data for troubleshooting.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body domain.DebugBundle }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "set-debug-mode",
		Method:      http.MethodPost,
		Path:        "/v1/runs/{runID}/debug",
		Summary:     "Set debug mode",
		Description: "Enables or disables debug mode for a run, increasing log verbosity.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  struct {
			Enabled bool `json:"enabled" doc:"Whether to enable debug mode" example:"true"`
		}
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-run-lineage",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/lineage",
		Summary:     "List run lineage",
		Description: "Returns the parent-child lineage tree for a run.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-run-dependency-status",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/dependency-status",
		Summary:     "Get run dependency status",
		Description: "Returns the status of all dependencies for a run.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "reset-idempotency-key",
		Method:      http.MethodDelete,
		Path:        "/v1/runs/{runID}/idempotency-key",
		Summary:     "Reset idempotency key",
		Description: "Clears the idempotency key for a run, allowing re-triggering.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "reschedule-run",
		Method:      http.MethodPost,
		Path:        "/v1/runs/{runID}/reschedule",
		Summary:     "Reschedule a run",
		Description: "Reschedules a queued run to execute at a different time.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
		Body  struct {
			ScheduledFor time.Time `json:"scheduled_for" required:"true" doc:"New scheduled time" example:"2024-01-15T09:00:00Z"`
		}
	}) (*struct{ Body domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "pause-run",
		Method:      http.MethodPost,
		Path:        "/v1/runs/{runID}/pause",
		Summary:     "Pause a run",
		Description: "Pauses an executing run at the next safe checkpoint.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "resume-run",
		Method:      http.MethodPost,
		Path:        "/v1/runs/{runID}/resume",
		Summary:     "Resume a run",
		Description: "Resumes a paused run from its last checkpoint.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "restart-run",
		Method:      http.MethodPost,
		Path:        "/v1/runs/{runID}/restart",
		Summary:     "Restart a run",
		Description: "Restarts a run from the beginning, discarding current progress.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body domain.JobRun }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-run-state",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/state",
		Summary:     "List run state",
		Description: "Returns all key-value pairs in the run's state store.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body []domain.RunState }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-run-llm-stream",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/stream/chunks",
		Summary:     "Get LLM stream chunks",
		Description: "Returns stored LLM streaming chunks for a run.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-run-resources",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/resources",
		Summary:     "List run resources",
		Description: "Returns resource utilization snapshots for a run.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body []domain.RunResourceSnapshot }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "stream-run",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/stream",
		Summary:     "Stream run updates",
		Description: "Opens an SSE stream for real-time updates on a run's execution.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerRegionOps registers region listing operations.

func (s *Server) registerRegionOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-regions",
		Method:      http.MethodGet,
		Path:        "/v1/regions",
		Summary:     "List available regions",
		Description: "Returns all available execution regions.",
		Tags:        []string{"Regions"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerStatsOps registers project statistics operations.

func (s *Server) registerStatsOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-stats",
		Method:      http.MethodGet,
		Path:        "/v1/stats",
		Summary:     "Get project statistics",
		Description: "Returns aggregate statistics for the current project including job and run counts.",
		Tags:        []string{"Stats"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerWorkflowPolicyOps registers workflow policy operations.

func (s *Server) registerWorkflowPolicyOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-workflow-policy",
		Method:      http.MethodGet,
		Path:        "/v1/workflow-policies/{projectID}",
		Summary:     "Get workflow policy",
		Description: "Returns the workflow execution policy for a project.",
		Tags:        []string{"Workflow Policies"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		ProjectID string `path:"projectID" doc:"Project ID" example:"proj_01HX7ZKRN5"`
	}) (*struct{ Body domain.WorkflowPolicy }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "upsert-workflow-policy",
		Method:      http.MethodPut,
		Path:        "/v1/workflow-policies/{projectID}",
		Summary:     "Create or update workflow policy",
		Description: "Creates or updates the workflow execution policy for a project.",
		Tags:        []string{"Workflow Policies"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 404, 500},
	}, func(_ context.Context, _ *struct {
		ProjectID string `path:"projectID" doc:"Project ID" example:"proj_01HX7ZKRN5"`
		Body      any
	}) (*struct{ Body domain.WorkflowPolicy }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}
