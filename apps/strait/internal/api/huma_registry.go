package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
)

// registerAllTypedOps registers Huma OpenAPI operations for all TypedHandler
// routes. Each call to RegisterTypedOp extracts the generic Input/Output types
// from the real handler function, so the OpenAPI spec exactly matches the
// actual request/response types -- no separate stub types needed.
//
//nolint:funlen // one registration call per API operation
func registerAllTypedOps(api huma.API, s *Server) {
	// -- CLI Auth --
	RegisterTypedOp(api, OpMeta{
		ID: "request-device-code", Method: http.MethodPost, Path: "/v1/cli/auth/device-code",
		Summary: "Request a device authorization code", Description: "Initiates the device authorization flow by generating a device code and user code.",
		Tags: []string{"CLI Auth"}, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleDeviceCode)

	RegisterTypedOp(api, OpMeta{
		ID: "poll-device-token", Method: http.MethodPost, Path: "/v1/cli/auth/token",
		Summary: "Poll for device token", Description: "Polls for the authorization token after the user approves the device code.",
		Tags: []string{"CLI Auth"}, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleDeviceToken)

	// -- Organizations --
	RegisterTypedOp(api, OpMeta{
		ID: "list-org-runs", Method: http.MethodGet, Path: "/v1/organizations/{orgID}/runs",
		Summary: "List runs across organization", Description: "Returns runs across all projects in an organization.",
		Tags: []string{"Organizations"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleListOrgRuns)

	RegisterTypedOp(api, OpMeta{
		ID: "list-org-jobs", Method: http.MethodGet, Path: "/v1/organizations/{orgID}/jobs",
		Summary: "List jobs across organization", Description: "Returns jobs across all projects in an organization.",
		Tags: []string{"Organizations"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleListOrgJobs)

	// -- Secrets --
	RegisterTypedOp(api, OpMeta{
		ID: "create-secret", Method: http.MethodPost, Path: "/v1/secrets",
		Summary: "Create a secret", Description: "Creates a new encrypted secret for use in job payloads.",
		Tags: []string{"Secrets"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500, 503},
	}, s.handleCreateSecret)

	RegisterTypedOp(api, OpMeta{
		ID: "list-secrets", Method: http.MethodGet, Path: "/v1/secrets",
		Summary: "List secrets", Description: "Returns all secrets in the current project. Values are redacted.",
		Tags: []string{"Secrets"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListSecrets)

	RegisterTypedOp(api, OpMeta{
		ID: "get-secret", Method: http.MethodGet, Path: "/v1/secrets/{secretID}",
		Summary: "Get a secret", Description: "Returns metadata for a single secret. The encrypted and decrypted values are never included.",
		Tags: []string{"Secrets"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetSecret)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-secret", Method: http.MethodDelete, Path: "/v1/secrets/{secretID}",
		Summary: "Delete a secret", Description: "Permanently deletes a secret.",
		Tags: []string{"Secrets"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleDeleteSecret)

	// -- Billing --
	RegisterTypedOp(api, OpMeta{
		ID: "list-plans", Method: http.MethodGet, Path: "/v1/plans",
		Summary: "List plans", Description: "Returns all available plan tiers with their launch limits, pricing, and roadmap metadata.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{401, 500},
	}, s.handleGetPlans)

	RegisterTypedOp(api, OpMeta{
		ID: "get-current-usage", Method: http.MethodGet, Path: "/v1/usage/current",
		Summary: "Get current usage", Description: "Returns the current billing period's usage metrics.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{401, 404, 500, 501},
	}, s.handleGetCurrentUsage)

	RegisterTypedOp(api, OpMeta{
		ID: "get-usage-history", Method: http.MethodGet, Path: "/v1/usage/history",
		Summary: "Get usage history", Description: "Returns historical usage data across billing periods.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{401, 404, 500, 501},
	}, s.handleGetUsageHistory)

	RegisterTypedOp(api, OpMeta{
		ID: "get-usage-forecast", Method: http.MethodGet, Path: "/v1/usage/forecast",
		Summary: "Get usage forecast", Description: "Returns projected usage for the current billing period.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{401, 404, 500, 501},
	}, s.handleGetUsageForecast)

	RegisterTypedOp(api, OpMeta{
		ID: "get-project-costs", Method: http.MethodGet, Path: "/v1/usage/projects",
		Summary: "Get project costs", Description: "Returns cost breakdown by project for the current billing period.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{401, 404, 500, 501},
	}, s.handleGetProjectCosts)

	RegisterTypedOp(api, OpMeta{
		ID: "get-anomaly-alerts", Method: http.MethodGet, Path: "/v1/usage/anomalies",
		Summary: "Get anomaly alerts", Description: "Returns usage anomaly alerts based on configured thresholds.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{401, 404, 500, 501},
	}, s.handleGetAnomalyAlerts)

	RegisterTypedOp(api, OpMeta{
		ID: "export-usage", Method: http.MethodGet, Path: "/v1/usage/export",
		Summary: "Export usage data", Description: "Exports usage data as CSV or JSON for external analysis.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500, 501},
	}, s.handleExportUsage)

	RegisterTypedOp(api, OpMeta{
		ID: "get-spending-limit", Method: http.MethodGet, Path: "/v1/spending-limit",
		Summary: "Get spending limit", Description: "Returns the current spending limit configuration.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500, 501},
	}, s.handleGetSpendingLimit)

	RegisterTypedOp(api, OpMeta{
		ID: "update-spending-limit", Method: http.MethodPut, Path: "/v1/spending-limit",
		Summary: "Update spending limit", Description: "Sets or updates the spending limit for the current project.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500, 501},
	}, s.handleUpdateSpendingLimit)

	RegisterTypedOp(api, OpMeta{
		ID: "get-downgrade-preview", Method: http.MethodGet, Path: "/v1/downgrade-preview",
		Summary: "Get downgrade preview", Description: "Returns a preview of the impact of downgrading to a lower plan tier.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500, 501},
	}, s.handleGetDowngradePreview)

	RegisterTypedOp(api, OpMeta{
		ID: "get-project-budget", Method: http.MethodGet, Path: "/v1/project-budget",
		Summary: "Get project budget", Description: "Returns the budget configuration for the current project.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500, 501},
	}, s.handleGetProjectBudget)

	RegisterTypedOp(api, OpMeta{
		ID: "update-project-budget", Method: http.MethodPut, Path: "/v1/project-budget",
		Summary: "Update project budget", Description: "Sets or updates the budget for the current project.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500, 501},
	}, s.handleUpdateProjectBudget)

	RegisterTypedOp(api, OpMeta{
		ID: "get-anomaly-config", Method: http.MethodGet, Path: "/v1/anomaly-config",
		Summary: "Get anomaly detection config", Description: "Returns the anomaly detection configuration for the current project.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500, 501},
	}, s.handleGetAnomalyConfig)

	RegisterTypedOp(api, OpMeta{
		ID: "update-anomaly-config", Method: http.MethodPut, Path: "/v1/anomaly-config",
		Summary: "Update anomaly detection config", Description: "Sets or updates the anomaly detection thresholds for the current project.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500, 501},
	}, s.handleUpdateAnomalyConfig)

	RegisterTypedOp(api, OpMeta{
		ID: "get-email-preferences", Method: http.MethodGet, Path: "/v1/usage/email-preferences",
		Summary: "Get email preferences", Description: "Returns email notification preferences for the organization.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{401, 500},
	}, s.handleGetEmailPreferences)

	RegisterTypedOp(api, OpMeta{
		ID: "update-email-preferences", Method: http.MethodPut, Path: "/v1/usage/email-preferences",
		Summary: "Update email preferences", Description: "Updates email notification preferences for the organization.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{400, 401, 500, 501},
	}, s.handleUpdateEmailPreferences)

	RegisterTypedOp(api, OpMeta{
		ID: "check-org-limit", Method: http.MethodGet, Path: "/v1/billing/check-org-limit",
		Summary: "Check organization limit", Description: "Checks whether the organization has reached its plan limits.",
		Tags: []string{"Billing"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleCheckOrgLimit)

	// -- Projects --
	RegisterTypedOp(api, OpMeta{
		ID: "create-project", Method: http.MethodPost, Path: "/v1/projects",
		Summary: "Create a project", Description: "Creates a new project within an organization.",
		Tags: []string{"Projects"}, Security: []map[string][]string{{"internalSecret": {}}}, Errors: []int{400, 401, 403, 409, 429, 500},
	}, s.handleCreateProject)

	RegisterTypedOp(api, OpMeta{
		ID: "list-projects", Method: http.MethodGet, Path: "/v1/projects",
		Summary: "List projects", Description: "Returns all projects accessible by the current API key.",
		Tags: []string{"Projects"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleListProjects)

	RegisterTypedOp(api, OpMeta{
		ID: "get-project", Method: http.MethodGet, Path: "/v1/projects/{projectID}",
		Summary: "Get a project", Description: "Returns details of a specific project.",
		Tags: []string{"Projects"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleGetProject)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-project", Method: http.MethodDelete, Path: "/v1/projects/{projectID}",
		Summary: "Delete a project", Description: "Permanently deletes a project and all its associated resources.",
		Tags: []string{"Projects"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleDeleteProject)

	RegisterTypedOp(api, OpMeta{
		ID: "get-project-settings", Method: http.MethodGet, Path: "/v1/projects/{projectID}/settings",
		Summary: "Get project settings", Description: "Returns the current settings for a project.",
		Tags: []string{"Projects"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleGetProjectSettings)

	RegisterTypedOp(api, OpMeta{
		ID: "update-project-settings", Method: http.MethodPut, Path: "/v1/projects/{projectID}/settings",
		Summary: "Update project settings", Description: "Updates the settings for a project.",
		Tags: []string{"Projects"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleUpdateProjectSettings)

	// -- Jobs --
	RegisterTypedOp(api, OpMeta{
		ID: "create-job", Method: http.MethodPost, Path: "/v1/jobs",
		Summary: "Create a job", Description: "Creates a new job with the specified endpoint and configuration.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateJob)

	RegisterTypedOp(api, OpMeta{
		ID: "list-jobs", Method: http.MethodGet, Path: "/v1/jobs",
		Summary: "List jobs", Description: "Returns a paginated list of jobs in the current project.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListJobs)

	RegisterTypedOp(api, OpMeta{
		ID: "batch-create-jobs", Method: http.MethodPost, Path: "/v1/jobs/batch",
		Summary: "Batch create jobs", Description: "Creates multiple jobs in a single request.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleBatchCreateJobs)

	RegisterTypedOp(api, OpMeta{
		ID: "batch-enable-jobs", Method: http.MethodPost, Path: "/v1/jobs/batch-enable",
		Summary: "Batch enable jobs", Description: "Enables multiple jobs in a single request.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleBatchEnableJobs)

	RegisterTypedOp(api, OpMeta{
		ID: "batch-disable-jobs", Method: http.MethodPost, Path: "/v1/jobs/batch-disable",
		Summary: "Batch disable jobs", Description: "Disables multiple jobs in a single request.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleBatchDisableJobs)

	RegisterTypedOp(api, OpMeta{
		ID: "get-job", Method: http.MethodGet, Path: "/v1/jobs/{jobID}",
		Summary: "Get a job", Description: "Returns details of a specific job.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetJob)

	RegisterTypedOp(api, OpMeta{
		ID: "update-job", Method: http.MethodPatch, Path: "/v1/jobs/{jobID}",
		Summary: "Update a job", Description: "Updates an existing job's configuration.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleUpdateJob)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-job", Method: http.MethodDelete, Path: "/v1/jobs/{jobID}",
		Summary: "Delete a job", Description: "Permanently deletes a job and all its associated data.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleDeleteJob)

	RegisterTypedOp(api, OpMeta{
		ID: "trigger-job", Method: http.MethodPost, Path: "/v1/jobs/{jobID}/trigger",
		Summary: "Trigger a job", Description: "Triggers immediate execution of a job.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 429, 500},
	}, s.handleTriggerJob)

	RegisterTypedOp(api, OpMeta{
		ID: "bulk-trigger-job", Method: http.MethodPost, Path: "/v1/jobs/{jobID}/trigger/bulk",
		Summary: "Bulk trigger a job", Description: "Triggers multiple executions of a job with different payloads.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 429, 500},
	}, s.handleBulkTriggerJob)

	RegisterTypedOp(api, OpMeta{
		ID: "create-job-dependency", Method: http.MethodPost, Path: "/v1/jobs/{jobID}/dependencies",
		Summary: "Create a job dependency", Description: "Creates a dependency relationship between two jobs.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 429, 500},
	}, s.handleCreateJobDependency)

	RegisterTypedOp(api, OpMeta{
		ID: "list-job-dependencies", Method: http.MethodGet, Path: "/v1/jobs/{jobID}/dependencies",
		Summary: "List job dependencies", Description: "Returns all dependencies for a specific job.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListJobDependencies)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-job-dependency", Method: http.MethodDelete, Path: "/v1/jobs/{jobID}/dependencies/{depID}",
		Summary: "Delete a job dependency", Description: "Removes a dependency relationship between two jobs.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleDeleteJobDependency)

	RegisterTypedOp(api, OpMeta{
		ID: "list-job-versions", Method: http.MethodGet, Path: "/v1/jobs/{jobID}/versions",
		Summary: "List job versions", Description: "Returns all versions of a job definition showing configuration history.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListJobVersions)

	RegisterTypedOp(api, OpMeta{
		ID: "get-job-version", Method: http.MethodGet, Path: "/v1/jobs/{jobID}/versions/{versionID}",
		Summary: "Get a job version", Description: "Returns details of a specific job version.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetJobVersion)

	RegisterTypedOp(api, OpMeta{
		ID: "clone-job", Method: http.MethodPost, Path: "/v1/jobs/{jobID}/clone",
		Summary: "Clone a job", Description: "Creates a copy of an existing job with a new name.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleCloneJob)

	RegisterTypedOp(api, OpMeta{
		ID: "get-job-health", Method: http.MethodGet, Path: "/v1/jobs/{jobID}/health",
		Summary: "Get job health", Description: "Returns health metrics for a job including success rate and latency.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetJobHealth)

	RegisterTypedOp(api, OpMeta{
		ID: "pause-job", Method: http.MethodPost, Path: "/v1/jobs/{jobID}/pause",
		Summary: "Pause a job", Description: "Pauses a job, preventing new runs from being dequeued while preserving queue state.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handlePauseJob)

	RegisterTypedOp(api, OpMeta{
		ID: "resume-job", Method: http.MethodPost, Path: "/v1/jobs/{jobID}/resume",
		Summary: "Resume a job", Description: "Resumes a paused job, allowing queued runs to be dequeued immediately.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleResumeJob)

	RegisterTypedOp(api, OpMeta{
		ID: "set-job-endpoint", Method: http.MethodPost, Path: "/v1/jobs/{jobID}/endpoint",
		Summary: "Set job endpoint", Description: "Sets the HTTP endpoint URL for a job and generates a fresh HMAC signing secret. SSRF-safe: private/loopback addresses are rejected at registration time.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleSetJobEndpoint)

	RegisterTypedOp(api, OpMeta{
		ID: "verify-job-endpoint", Method: http.MethodPost, Path: "/v1/jobs/{jobID}/endpoint/verify",
		Summary: "Verify job endpoint", Description: "Sends a signed HMAC test ping to the job's configured endpoint URL and returns the outcome.",
		Tags: []string{"Jobs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 429, 500},
	}, s.handleVerifyJobEndpoint)

	// -- Job Groups --
	RegisterTypedOp(api, OpMeta{
		ID: "create-job-group", Method: http.MethodPost, Path: "/v1/job-groups",
		Summary: "Create a job group", Description: "Creates a new group for organizing related jobs.",
		Tags: []string{"Job Groups"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateJobGroup)

	RegisterTypedOp(api, OpMeta{
		ID: "list-job-groups", Method: http.MethodGet, Path: "/v1/job-groups",
		Summary: "List job groups", Description: "Returns all job groups in the current project.",
		Tags: []string{"Job Groups"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListJobGroups)

	RegisterTypedOp(api, OpMeta{
		ID: "get-job-group", Method: http.MethodGet, Path: "/v1/job-groups/{groupID}",
		Summary: "Get a job group", Description: "Returns details of a specific job group.",
		Tags: []string{"Job Groups"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetJobGroup)

	RegisterTypedOp(api, OpMeta{
		ID: "update-job-group", Method: http.MethodPatch, Path: "/v1/job-groups/{groupID}",
		Summary: "Update a job group", Description: "Updates an existing job group's name or configuration.",
		Tags: []string{"Job Groups"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleUpdateJobGroup)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-job-group", Method: http.MethodDelete, Path: "/v1/job-groups/{groupID}",
		Summary: "Delete a job group", Description: "Permanently deletes a job group. Jobs in the group are not deleted.",
		Tags: []string{"Job Groups"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleDeleteJobGroup)

	RegisterTypedOp(api, OpMeta{
		ID: "list-jobs-by-group", Method: http.MethodGet, Path: "/v1/job-groups/{groupID}/jobs",
		Summary: "List jobs in a group", Description: "Returns all jobs belonging to a specific job group.",
		Tags: []string{"Job Groups"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListJobsByGroup)

	RegisterTypedOp(api, OpMeta{
		ID: "pause-all-jobs-by-group", Method: http.MethodPost, Path: "/v1/job-groups/{groupID}/pause-all",
		Summary: "Pause all jobs in a group", Description: "Pauses all jobs belonging to a specific job group.",
		Tags: []string{"Job Groups"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handlePauseAllJobsByGroup)

	RegisterTypedOp(api, OpMeta{
		ID: "resume-all-jobs-by-group", Method: http.MethodPost, Path: "/v1/job-groups/{groupID}/resume-all",
		Summary: "Resume all jobs in a group", Description: "Resumes all paused jobs belonging to a specific job group.",
		Tags: []string{"Job Groups"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleResumeAllJobsByGroup)

	RegisterTypedOp(api, OpMeta{
		ID: "get-job-group-stats", Method: http.MethodGet, Path: "/v1/job-groups/{groupID}/stats",
		Summary: "Get job group statistics", Description: "Returns aggregate statistics for all jobs in a group.",
		Tags: []string{"Job Groups"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleGetJobGroupStats)

	// -- Environments --
	RegisterTypedOp(api, OpMeta{
		ID: "create-environment", Method: http.MethodPost, Path: "/v1/environments",
		Summary: "Create an environment", Description: "Creates a new environment for isolating job configurations.",
		Tags: []string{"Environments"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateEnvironment)

	RegisterTypedOp(api, OpMeta{
		ID: "list-environments", Method: http.MethodGet, Path: "/v1/environments",
		Summary: "List environments", Description: "Returns all environments in the current project.",
		Tags: []string{"Environments"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListEnvironments)

	RegisterTypedOp(api, OpMeta{
		ID: "get-environment", Method: http.MethodGet, Path: "/v1/environments/{envID}",
		Summary: "Get an environment", Description: "Returns details of a specific environment.",
		Tags: []string{"Environments"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetEnvironment)

	RegisterTypedOp(api, OpMeta{
		ID: "update-environment", Method: http.MethodPatch, Path: "/v1/environments/{envID}",
		Summary: "Update an environment", Description: "Updates an existing environment's configuration.",
		Tags: []string{"Environments"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleUpdateEnvironment)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-environment", Method: http.MethodDelete, Path: "/v1/environments/{envID}",
		Summary: "Delete an environment", Description: "Permanently deletes an environment.",
		Tags: []string{"Environments"}, Security: bearerSecurity, Errors: []int{401, 403, 404, 500},
	}, s.handleDeleteEnvironment)

	RegisterTypedOp(api, OpMeta{
		ID: "get-resolved-variables", Method: http.MethodGet, Path: "/v1/environments/{envID}/variables",
		Summary: "Get resolved variables", Description: "Returns the resolved environment variables with inheritance applied.",
		Tags: []string{"Environments"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleGetResolvedVariables)

	// -- Admin DLQ --
	RegisterTypedOp(api, OpMeta{
		ID: "admin-list-dlq", Method: http.MethodGet, Path: "/v1/admin/dlq",
		Summary: "List dead-letter runs (admin)", Description: "Admin-only paginated listing of dead-letter runs with optional job_id and masked filters.",
		Tags: []string{"Admin DLQ"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleAdminListDLQ)

	RegisterTypedOp(api, OpMeta{
		ID: "admin-replay-dlq", Method: http.MethodPost, Path: "/v1/admin/dlq/{run_id}/replay",
		Summary: "Replay a dead-letter run (admin)", Description: "Re-enqueues a dead-letter run via the admin path and records an audit event. Requires the dlq:replay scope.",
		Tags: []string{"Admin DLQ"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 409, 500},
	}, s.handleAdminReplayDLQ)

	RegisterTypedOp(api, OpMeta{
		ID: "admin-unmask-dlq", Method: http.MethodPost, Path: "/v1/admin/dlq/{run_id}/unmask",
		Summary: "Unmask a dead-letter run (admin)", Description: "Clears visible_until on a dead-letter run so it is no longer hidden by the age-out masker. Requires the dlq:replay scope.",
		Tags: []string{"Admin DLQ"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 409, 500},
	}, s.handleAdminUnmaskDLQ)

	RegisterTypedOp(api, OpMeta{
		ID: "admin-purge-dlq", Method: http.MethodPost, Path: "/v1/admin/dlq/{run_id}/purge",
		Summary: "Purge a dead-letter run (admin)", Description: "Hard-deletes a dead-letter run. Requires the dlq:purge scope.",
		Tags: []string{"Admin DLQ"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 409, 500},
	}, s.handleAdminPurgeDLQ)

	RegisterTypedOp(api, OpMeta{
		ID: "admin-list-outbox", Method: http.MethodGet, Path: "/v1/admin/outbox",
		Summary: "List quarantined outbox rows (admin)", Description: "Read-only paginated listing of terminal outbox rows that were quarantined after enqueue promotion failed.",
		Tags: []string{"Admin Outbox"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500, 503},
	}, s.handleAdminListOutbox)

	RegisterTypedOp(api, OpMeta{
		ID: "admin-get-outbox", Method: http.MethodGet, Path: "/v1/admin/outbox/{outbox_id}",
		Summary: "Get a quarantined outbox row (admin)", Description: "Returns one quarantined outbox row including the stored terminal error text.",
		Tags: []string{"Admin Outbox"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500, 503},
	}, s.handleAdminGetOutbox)

	RegisterTypedOp(api, OpMeta{
		ID: "admin-retry-outbox", Method: http.MethodPost, Path: "/v1/admin/outbox/{outbox_id}/retry",
		Summary: "Retry a quarantined outbox row (admin)", Description: "Creates a fresh retry clone from a quarantined outbox row and records an audit event. Requires the outbox:retry scope.",
		Tags: []string{"Admin Outbox"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 409, 500, 503},
	}, s.handleAdminRetryOutbox)

	RegisterTypedOp(api, OpMeta{
		ID: "admin-purge-outbox", Method: http.MethodPost, Path: "/v1/admin/outbox/{outbox_id}/purge",
		Summary: "Purge a quarantined outbox row (admin)", Description: "Hard-deletes a quarantined outbox row and records an audit event. Requires the outbox:purge scope.",
		Tags: []string{"Admin Outbox"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 409, 500, 503},
	}, s.handleAdminPurgeOutbox)

	// -- Runs --
	RegisterTypedOp(api, OpMeta{
		ID: "list-runs", Method: http.MethodGet, Path: "/v1/runs",
		Summary: "List runs", Description: "Returns a paginated list of job runs.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListRuns)

	RegisterTypedOp(api, OpMeta{
		ID: "list-dead-letter-runs", Method: http.MethodGet, Path: "/v1/runs/dlq",
		Summary: "List dead-letter queue runs", Description: "Returns runs that have exhausted all retry attempts.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListDeadLetterRuns)

	RegisterTypedOp(api, OpMeta{
		ID: "bulk-replay-dead-letter-runs", Method: http.MethodPost, Path: "/v1/runs/bulk-dlq-replay",
		Summary: "Bulk replay dead-letter runs", Description: "Replays multiple runs from the dead-letter queue.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleBulkReplayDeadLetterRuns)

	RegisterTypedOp(api, OpMeta{
		ID: "bulk-cancel-runs", Method: http.MethodPost, Path: "/v1/runs/bulk-cancel",
		Summary: "Bulk cancel runs", Description: "Cancels multiple runs matching the provided IDs.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleBulkCancelRuns)

	RegisterTypedOp(api, OpMeta{
		ID: "bulk-cancel-all-runs", Method: http.MethodPost, Path: "/v1/runs/bulk-cancel-all",
		Summary: "Bulk cancel all active runs", Description: "Cancels all active runs in the current project.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleBulkCancelAll)

	RegisterTypedOp(api, OpMeta{
		ID: "bulk-replay-runs", Method: http.MethodPost, Path: "/v1/runs/bulk-replay",
		Summary: "Bulk replay runs", Description: "Replays multiple runs matching the provided IDs.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleBulkReplayRuns)

	RegisterTypedOp(api, OpMeta{
		ID: "get-run", Method: http.MethodGet, Path: "/v1/runs/{runID}",
		Summary: "Get a run", Description: "Returns details of a specific job run.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleGetRun)

	RegisterTypedOp(api, OpMeta{
		ID: "cancel-run", Method: http.MethodDelete, Path: "/v1/runs/{runID}",
		Summary: "Cancel a run", Description: "Cancels a queued or executing run.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleCancelRun)

	RegisterTypedOp(api, OpMeta{
		ID: "replay-run", Method: http.MethodPost, Path: "/v1/runs/{runID}/replay",
		Summary: "Replay a run", Description: "Creates a new run with the same configuration as the original.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleReplayRun)

	RegisterTypedOp(api, OpMeta{
		ID: "replay-dead-letter-run", Method: http.MethodPost, Path: "/v1/runs/{runID}/dlq-replay",
		Summary: "Replay a dead-letter run", Description: "Replays a single run from the dead-letter queue.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleReplayDeadLetterRun)

	RegisterTypedOp(api, OpMeta{
		ID: "list-child-runs", Method: http.MethodGet, Path: "/v1/runs/{runID}/children",
		Summary: "List child runs", Description: "Returns all child runs spawned by the specified run.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListChildRuns)

	RegisterTypedOp(api, OpMeta{
		ID: "list-run-events", Method: http.MethodGet, Path: "/v1/runs/{runID}/events",
		Summary: "List run events", Description: "Returns the event log for a specific run.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListRunEvents)

	RegisterTypedOp(api, OpMeta{
		ID: "list-run-checkpoints", Method: http.MethodGet, Path: "/v1/runs/{runID}/checkpoints",
		Summary: "List run checkpoints", Description: "Returns all checkpoints saved during a run's execution.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListRunCheckpoints)

	RegisterTypedOp(api, OpMeta{
		ID: "list-run-outputs", Method: http.MethodGet, Path: "/v1/runs/{runID}/outputs",
		Summary: "List run outputs", Description: "Returns all structured outputs produced by a run.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListRunOutputs)

	RegisterTypedOp(api, OpMeta{
		ID: "get-debug-bundle", Method: http.MethodGet, Path: "/v1/runs/{runID}/debug-bundle",
		Summary: "Get debug bundle", Description: "Returns a comprehensive debug bundle with all run data for troubleshooting.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetDebugBundle)

	RegisterTypedOp(api, OpMeta{
		ID: "set-debug-mode", Method: http.MethodPost, Path: "/v1/runs/{runID}/debug",
		Summary: "Set debug mode", Description: "Enables or disables debug mode for a run, increasing log verbosity.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleSetDebugMode)

	RegisterTypedOp(api, OpMeta{
		ID: "list-run-lineage", Method: http.MethodGet, Path: "/v1/runs/{runID}/lineage",
		Summary: "List run lineage", Description: "Returns the parent-child lineage tree for a run.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListRunLineage)

	RegisterTypedOp(api, OpMeta{
		ID: "get-run-dependency-status", Method: http.MethodGet, Path: "/v1/runs/{runID}/dependency-status",
		Summary: "Get run dependency status", Description: "Returns the status of all dependencies for a run.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetRunDependencyStatus)

	RegisterTypedOp(api, OpMeta{
		ID: "reset-idempotency-key", Method: http.MethodDelete, Path: "/v1/runs/{runID}/idempotency-key",
		Summary: "Reset idempotency key", Description: "Clears the idempotency key for a run, allowing re-triggering.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleResetIdempotencyKey)

	RegisterTypedOp(api, OpMeta{
		ID: "reschedule-run", Method: http.MethodPost, Path: "/v1/runs/{runID}/reschedule",
		Summary: "Reschedule a run", Description: "Reschedules a queued run to execute at a different time.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleRescheduleRun)

	RegisterTypedOp(api, OpMeta{
		ID: "pause-run", Method: http.MethodPost, Path: "/v1/runs/{runID}/pause",
		Summary: "Pause a run", Description: "Pauses an executing run at the next safe checkpoint.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handlePauseRun)

	RegisterTypedOp(api, OpMeta{
		ID: "resume-run", Method: http.MethodPost, Path: "/v1/runs/{runID}/resume",
		Summary: "Resume a run", Description: "Resumes a paused run from its last checkpoint.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleResumeRun)

	RegisterTypedOp(api, OpMeta{
		ID: "restart-run", Method: http.MethodPost, Path: "/v1/runs/{runID}/restart",
		Summary: "Restart a run", Description: "Restarts a run from the beginning, discarding current progress.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleRestartRun)

	RegisterTypedOp(api, OpMeta{
		ID: "list-run-state", Method: http.MethodGet, Path: "/v1/runs/{runID}/state",
		Summary: "List run state", Description: "Returns all key-value pairs in the run's state store.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListRunState)

	RegisterTypedOp(api, OpMeta{
		ID: "list-run-resources", Method: http.MethodGet, Path: "/v1/runs/{runID}/resources",
		Summary: "List run resources", Description: "Returns resource utilization snapshots for a run.",
		Tags: []string{"Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListRunResources)

	// -- Batch Operations --
	RegisterTypedOp(api, OpMeta{
		ID: "list-batch-operations", Method: http.MethodGet, Path: "/v1/batch-operations",
		Summary: "List batch operations", Description: "Returns a paginated list of batch operations and their progress.",
		Tags: []string{"Batch Operations"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListBatchOperations)

	RegisterTypedOp(api, OpMeta{
		ID: "get-batch-operation", Method: http.MethodGet, Path: "/v1/batch-operations/{batchID}",
		Summary: "Get a batch operation", Description: "Returns details and progress of a specific batch operation.",
		Tags: []string{"Batch Operations"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetBatchOperation)

	// -- Workers --
	RegisterTypedOp(api, OpMeta{
		ID: "list-workers", Method: http.MethodGet, Path: "/v1/workers",
		Summary: "List workers", Description: "Returns a paginated list of connected and recently-seen workers for the current project.",
		Tags: []string{"Workers"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleListWorkers)

	RegisterTypedOp(api, OpMeta{
		ID: "get-worker", Method: http.MethodGet, Path: "/v1/workers/{workerID}",
		Summary: "Get a worker", Description: "Returns details of a specific worker. Returns 404 for workers in other projects to avoid existence leaks.",
		Tags: []string{"Workers"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleGetWorker)

	RegisterTypedOp(api, OpMeta{
		ID: "force-disconnect-worker", Method: http.MethodDelete, Path: "/v1/workers/{workerID}",
		Summary: "Force-disconnect a worker", Description: "Publishes a disconnect signal to the owning replica and waits for worker-plane acknowledgement. Returns 503 with Retry-After if the disconnect is still pending. Returns 404 for workers in other projects.",
		Tags: []string{"Workers"}, Security: bearerSecurity, Errors: []int{401, 404, 500, 503},
	}, s.handleDeleteWorker)

	RegisterTypedOp(api, OpMeta{
		ID: "list-worker-tasks", Method: http.MethodGet, Path: "/v1/workers/{workerID}/tasks",
		Summary: "List worker tasks", Description: "Returns a paginated list of run tasks assigned to a specific worker.",
		Tags: []string{"Workers"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListWorkerTasks)

	// -- Webhooks (legacy top-level routes) --
	RegisterTypedOp(api, OpMeta{
		ID: "list-webhook-deliveries-legacy", Method: http.MethodGet, Path: "/v1/webhook-deliveries",
		Summary: "List webhook deliveries (legacy)", Description: "Returns a paginated list of webhook delivery attempts. Legacy endpoint, prefer /v1/webhooks/deliveries.",
		Tags: []string{"Webhooks"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListWebhookDeliveries)

	// Registered manually (not via RegisterTypedOp) because the handler's input
	// struct declares both DeliveryID and ID to support two path variants. Using
	// RegisterTypedOp would leak the wrong parameter into each route's spec.
	huma.Register(api, huma.Operation{
		OperationID: "retry-webhook-delivery-legacy",
		Method:      http.MethodPost,
		Path:        "/v1/webhook-deliveries/{deliveryID}/retry",
		Summary:     "Retry a webhook delivery (legacy)",
		Description: "Retries a failed webhook delivery. Legacy endpoint, prefer /v1/webhooks/deliveries/{id}/retry.",
		Tags:        []string{"Webhooks"},
		Security:    bearerSecurity,
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		DeliveryID string `path:"deliveryID" doc:"Webhook delivery ID" example:"whd_01HX8BQNP4"`
	}) (*struct {
		Body *domain.WebhookDelivery
	}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	// -- Webhooks --
	RegisterTypedOp(api, OpMeta{
		ID: "test-webhook", Method: http.MethodPost, Path: "/v1/webhooks/test",
		Summary: "Test a webhook", Description: "Sends a test payload to a webhook URL to verify connectivity.",
		Tags: []string{"Webhooks"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleTestWebhook)

	RegisterTypedOp(api, OpMeta{
		ID: "list-webhook-deliveries", Method: http.MethodGet, Path: "/v1/webhooks/deliveries",
		Summary: "List webhook deliveries", Description: "Returns a paginated list of webhook delivery attempts.",
		Tags: []string{"Webhooks"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListWebhookDeliveries)

	RegisterTypedOp(api, OpMeta{
		ID: "get-webhook-delivery", Method: http.MethodGet, Path: "/v1/webhooks/deliveries/{id}",
		Summary: "Get a webhook delivery", Description: "Returns details of a specific webhook delivery attempt.",
		Tags: []string{"Webhooks"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetWebhookDelivery)

	huma.Register(api, huma.Operation{
		OperationID: "retry-webhook-delivery",
		Method:      http.MethodPost,
		Path:        "/v1/webhooks/deliveries/{id}/retry",
		Summary:     "Retry a webhook delivery",
		Description: "Retries a failed webhook delivery attempt.",
		Tags:        []string{"Webhooks"},
		Security:    bearerSecurity,
		Errors:      []int{400, 401, 404, 409, 500},
	}, func(_ context.Context, _ *struct {
		ID string `path:"id" doc:"Webhook delivery ID" example:"whd_01HX8BQNP4"`
	}) (*struct {
		Body *domain.WebhookDelivery
	}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	RegisterTypedOp(api, OpMeta{
		ID: "replay-webhook-delivery", Method: http.MethodPost, Path: "/v1/webhooks/deliveries/{id}/replay",
		Summary: "Replay a webhook delivery", Description: "Replays a webhook delivery with the original payload.",
		Tags: []string{"Webhooks"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleReplayWebhookDelivery)

	RegisterTypedOp(api, OpMeta{
		ID: "create-webhook-subscription", Method: http.MethodPost, Path: "/v1/webhooks/subscriptions",
		Summary: "Create a webhook subscription", Description: "Creates a new webhook subscription to receive event notifications.",
		Tags: []string{"Webhooks"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateWebhookSubscription)

	RegisterTypedOp(api, OpMeta{
		ID: "list-webhook-subscriptions", Method: http.MethodGet, Path: "/v1/webhooks/subscriptions",
		Summary: "List webhook subscriptions", Description: "Returns all webhook subscriptions in the current project.",
		Tags: []string{"Webhooks"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListWebhookSubscriptions)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-webhook-subscription", Method: http.MethodDelete, Path: "/v1/webhooks/subscriptions/{id}",
		Summary: "Delete a webhook subscription", Description: "Removes a webhook subscription, stopping further deliveries.",
		Tags: []string{"Webhooks"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleDeleteWebhookSubscription)

	RegisterTypedOp(api, OpMeta{
		ID: "rotate-webhook-secret", Method: http.MethodPost, Path: "/v1/webhooks/subscriptions/{id}/rotate-secret",
		Summary: "Rotate webhook signing secret", Description: "Rotates the HMAC signing secret for a webhook subscription with a grace period during which both old and new signatures are sent.",
		Tags: []string{"Webhooks"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleRotateWebhookSecret)

	// -- Notifications --
	RegisterTypedOp(api, OpMeta{
		ID: "create-notification-channel", Method: http.MethodPost, Path: "/v1/notification-channels",
		Summary: "Create a notification channel", Description: "Creates a new notification channel for receiving alerts about job events.",
		Tags: []string{"Notifications"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateNotificationChannel)

	RegisterTypedOp(api, OpMeta{
		ID: "list-notification-channels", Method: http.MethodGet, Path: "/v1/notification-channels",
		Summary: "List notification channels", Description: "Returns all notification channels in the current project.",
		Tags: []string{"Notifications"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListNotificationChannels)

	RegisterTypedOp(api, OpMeta{
		ID: "get-notification-channel", Method: http.MethodGet, Path: "/v1/notification-channels/{channelID}",
		Summary: "Get a notification channel", Description: "Returns details of a specific notification channel.",
		Tags: []string{"Notifications"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetNotificationChannel)

	RegisterTypedOp(api, OpMeta{
		ID: "update-notification-channel", Method: http.MethodPatch, Path: "/v1/notification-channels/{channelID}",
		Summary: "Update a notification channel", Description: "Updates an existing notification channel's configuration.",
		Tags: []string{"Notifications"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleUpdateNotificationChannel)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-notification-channel", Method: http.MethodDelete, Path: "/v1/notification-channels/{channelID}",
		Summary: "Delete a notification channel", Description: "Permanently deletes a notification channel.",
		Tags: []string{"Notifications"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleDeleteNotificationChannel)

	RegisterTypedOp(api, OpMeta{
		ID: "list-notification-deliveries", Method: http.MethodGet, Path: "/v1/notification-deliveries",
		Summary: "List notification deliveries", Description: "Returns a paginated list of notification delivery attempts.",
		Tags: []string{"Notifications"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListNotificationDeliveries)

	// -- Log Drains --
	RegisterTypedOp(api, OpMeta{
		ID: "list-log-drains", Method: http.MethodGet, Path: "/v1/log-drains",
		Summary: "List log drains", Description: "Returns all log drains configured in the current project.",
		Tags: []string{"Log Drains"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleListLogDrains)

	RegisterTypedOp(api, OpMeta{
		ID: "create-log-drain", Method: http.MethodPost, Path: "/v1/log-drains",
		Summary: "Create a log drain", Description: "Creates a new log drain to forward job execution logs to an external destination.",
		Tags: []string{"Log Drains"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateLogDrain)

	RegisterTypedOp(api, OpMeta{
		ID: "get-log-drain", Method: http.MethodGet, Path: "/v1/log-drains/{drainID}",
		Summary: "Get a log drain", Description: "Returns details of a specific log drain.",
		Tags: []string{"Log Drains"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetLogDrain)

	RegisterTypedOp(api, OpMeta{
		ID: "update-log-drain", Method: http.MethodPatch, Path: "/v1/log-drains/{drainID}",
		Summary: "Update a log drain", Description: "Updates an existing log drain's configuration.",
		Tags: []string{"Log Drains"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleUpdateLogDrain)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-log-drain", Method: http.MethodDelete, Path: "/v1/log-drains/{drainID}",
		Summary: "Delete a log drain", Description: "Permanently deletes a log drain.",
		Tags: []string{"Log Drains"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleDeleteLogDrain)

	// -- API Keys --
	RegisterTypedOp(api, OpMeta{
		ID: "create-api-key", Method: http.MethodPost, Path: "/v1/api-keys",
		Summary: "Create an API key", Description: "Creates a new API key for authenticating with the API.",
		Tags: []string{"API Keys"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateAPIKey)

	RegisterTypedOp(api, OpMeta{
		ID: "list-api-keys", Method: http.MethodGet, Path: "/v1/api-keys",
		Summary: "List API keys", Description: "Returns all API keys for the current project.",
		Tags: []string{"API Keys"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListAPIKeys)

	RegisterTypedOp(api, OpMeta{
		ID: "rotate-api-key", Method: http.MethodPost, Path: "/v1/api-keys/{keyID}/rotate",
		Summary: "Rotate an API key", Description: "Rotates an API key, generating a new secret while keeping the same ID.",
		Tags: []string{"API Keys"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleRotateAPIKey)

	RegisterTypedOp(api, OpMeta{
		ID: "revoke-api-key", Method: http.MethodDelete, Path: "/v1/api-keys/{keyID}",
		Summary: "Revoke an API key", Description: "Permanently revokes an API key, immediately invalidating it.",
		Tags: []string{"API Keys"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleRevokeAPIKey)

	RegisterTypedOp(api, OpMeta{
		ID: "list-expiring-keys", Method: http.MethodGet, Path: "/v1/api-keys/expiring-soon",
		Summary: "List expiring API keys", Description: "Returns API keys that are expiring within the specified number of days.",
		Tags: []string{"API Keys"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleListExpiringKeys)

	// -- CLI Auth (approve) --
	RegisterTypedOp(api, OpMeta{
		ID: "approve-device-code", Method: http.MethodPost, Path: "/v1/cli/device-codes/approve",
		Summary: "Approve a device code", Description: "Approves a pending device code, authorizing the CLI session.",
		Tags: []string{"CLI Auth"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleApproveDeviceCode)

	// -- Stats --
	RegisterTypedOp(api, OpMeta{
		ID: "get-stats", Method: http.MethodGet, Path: "/v1/stats",
		Summary: "Get project statistics", Description: "Returns aggregate statistics for the current project including job and run counts.",
		Tags: []string{"Stats"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleStats)

	RegisterTypedOp(api, OpMeta{
		ID: "create-sse-token", Method: http.MethodPost, Path: "/v1/sse-token",
		Summary: "Create SSE token", Description: "Issues a short-lived JWT for use as a query-param token in SSE endpoints.",
		Tags: []string{"Authentication"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleCreateSSEToken)

	// -- Analytics (Community, Postgres-backed) --
	RegisterTypedOp(api, OpMeta{
		ID: "get-performance-analytics", Method: http.MethodGet, Path: "/v1/analytics/performance",
		Summary: "Get performance analytics", Description: "Returns job execution performance metrics including p50/p95/p99 latencies.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetPerformanceAnalytics)

	RegisterTypedOp(api, OpMeta{
		ID: "get-cost-analytics", Method: http.MethodGet, Path: "/v1/analytics/costs",
		Summary: "Get cost analytics", Description: "Returns cost analytics for the current billing period.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetCostAnalytics)

	RegisterTypedOp(api, OpMeta{
		ID: "get-cost-trends", Method: http.MethodGet, Path: "/v1/analytics/costs/trends",
		Summary: "Get cost trends", Description: "Returns cost trend data over time for identifying spending patterns.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleGetCostTrends)

	RegisterTypedOp(api, OpMeta{
		ID: "get-top-costs", Method: http.MethodGet, Path: "/v1/analytics/costs/top",
		Summary: "Get top cost contributors", Description: "Returns the jobs contributing most to overall costs.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetTopCosts)

	RegisterTypedOp(api, OpMeta{
		ID: "get-approval-stats", Method: http.MethodGet, Path: "/v1/analytics/approvals",
		Summary: "Get approval statistics", Description: "Returns statistics about workflow approval steps including wait times.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleGetApprovalStats)

	RegisterTypedOp(api, OpMeta{
		ID: "get-cost-insights", Method: http.MethodGet, Path: "/v1/analytics/cost-insights",
		Summary: "Get cost insights", Description: "Returns actionable insights for reducing costs based on usage patterns.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetCostInsights)

	// -- Analytics (Cloud-only, ClickHouse-backed) --
	RegisterTypedOp(api, OpMeta{
		ID: "get-run-timeline", Method: http.MethodGet, Path: "/v1/analytics/runs/timeline",
		Summary: "Get run timeline", Description: "Returns time-series data of run executions bucketed by interval.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleRunTimeline)

	RegisterTypedOp(api, OpMeta{
		ID: "get-run-duration-distribution", Method: http.MethodGet, Path: "/v1/analytics/runs/duration-distribution",
		Summary: "Get run duration distribution", Description: "Returns a histogram of run durations across percentile buckets.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleRunDurationDistribution)

	RegisterTypedOp(api, OpMeta{
		ID: "get-run-failure-reasons", Method: http.MethodGet, Path: "/v1/analytics/runs/failure-reasons",
		Summary: "Get run failure reasons", Description: "Returns the most common failure reasons across runs.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleRunFailureReasons)

	RegisterTypedOp(api, OpMeta{
		ID: "get-run-summary", Method: http.MethodGet, Path: "/v1/analytics/runs/summary",
		Summary: "Get run summary", Description: "Returns aggregate summary statistics for runs in the specified period.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleRunSummary)

	RegisterTypedOp(api, OpMeta{
		ID: "get-runs-by-trigger", Method: http.MethodGet, Path: "/v1/analytics/runs/by-trigger",
		Summary: "Get runs by trigger type", Description: "Returns run counts grouped by trigger type (cron, API, event, webhook).",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleRunsByTrigger)

	RegisterTypedOp(api, OpMeta{
		ID: "get-job-comparison", Method: http.MethodGet, Path: "/v1/analytics/jobs/comparison",
		Summary: "Get job comparison", Description: "Returns side-by-side performance comparison across jobs.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleJobComparison)

	RegisterTypedOp(api, OpMeta{
		ID: "get-job-reliability", Method: http.MethodGet, Path: "/v1/analytics/jobs/reliability",
		Summary: "Get job reliability", Description: "Returns reliability metrics (success rate, MTTR, MTBF) for jobs.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleJobReliability)

	RegisterTypedOp(api, OpMeta{
		ID: "get-runs-by-version", Method: http.MethodGet, Path: "/v1/analytics/jobs/by-version",
		Summary: "Get runs by job version", Description: "Returns run metrics grouped by job version to track version performance.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleRunsByVersion)

	RegisterTypedOp(api, OpMeta{
		ID: "get-job-cost-ranking", Method: http.MethodGet, Path: "/v1/analytics/jobs/cost-ranking",
		Summary: "Get job cost ranking", Description: "Returns jobs ranked by total cost in the specified period.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleJobCostRanking)

	RegisterTypedOp(api, OpMeta{
		ID: "get-top-failing-jobs", Method: http.MethodGet, Path: "/v1/analytics/jobs/top-failing",
		Summary: "Get top failing jobs", Description: "Returns jobs with the highest failure rates in the specified period.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleTopFailingJobs)

	RegisterTypedOp(api, OpMeta{
		ID: "get-job-history", Method: http.MethodGet, Path: "/v1/analytics/jobs/{jobID}/history",
		Summary: "Get job execution history", Description: "Returns historical execution data for a specific job.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleJobHistory)

	RegisterTypedOp(api, OpMeta{
		ID: "get-tag-summary", Method: http.MethodGet, Path: "/v1/analytics/tags/summary",
		Summary: "Get tag summary", Description: "Returns aggregate metrics grouped by tag for resource categorization.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleTagSummary)

	RegisterTypedOp(api, OpMeta{
		ID: "get-top-failing-tags", Method: http.MethodGet, Path: "/v1/analytics/tags/top-failing",
		Summary: "Get top failing tags", Description: "Returns tags associated with the highest failure rates.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleTopFailingTags)

	RegisterTypedOp(api, OpMeta{
		ID: "get-tag-cost", Method: http.MethodGet, Path: "/v1/analytics/tags/cost",
		Summary: "Get tag cost breakdown", Description: "Returns cost data grouped by tag for cost allocation.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleTagCost)

	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-completion-rates", Method: http.MethodGet, Path: "/v1/analytics/workflows/completion-rates",
		Summary: "Get workflow completion rates", Description: "Returns completion and success rates for workflows.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleWorkflowCompletionRates)

	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-analytics-summary", Method: http.MethodGet, Path: "/v1/analytics/workflows/summary",
		Summary: "Get workflow analytics summary", Description: "Returns aggregate analytics across all workflows.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleWorkflowAnalyticsSummary)

	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-step-durations", Method: http.MethodGet, Path: "/v1/analytics/workflows/{workflowID}/step-durations",
		Summary: "Get workflow step durations", Description: "Returns average and percentile duration metrics for each step in a workflow.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleWorkflowStepDurations)

	RegisterTypedOp(api, OpMeta{
		ID: "get-webhook-delivery-stats", Method: http.MethodGet, Path: "/v1/analytics/webhooks/delivery-stats",
		Summary: "Get webhook delivery statistics", Description: "Returns delivery success rates and latency metrics for webhooks.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleWebhookDeliveryStats)

	RegisterTypedOp(api, OpMeta{
		ID: "get-webhook-endpoint-health", Method: http.MethodGet, Path: "/v1/analytics/webhooks/endpoint-health",
		Summary: "Get webhook endpoint health", Description: "Returns health metrics for webhook endpoints including error rates.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleWebhookEndpointHealth)

	RegisterTypedOp(api, OpMeta{
		ID: "get-top-failing-webhooks", Method: http.MethodGet, Path: "/v1/analytics/webhooks/top-failing",
		Summary: "Get top failing webhooks", Description: "Returns webhook endpoints with the highest failure rates.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleTopFailingWebhooks)

	RegisterTypedOp(api, OpMeta{
		ID: "get-event-volume", Method: http.MethodGet, Path: "/v1/analytics/events/volume",
		Summary: "Get event volume", Description: "Returns event volume time-series data.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleEventVolume)

	RegisterTypedOp(api, OpMeta{
		ID: "get-event-latency", Method: http.MethodGet, Path: "/v1/analytics/events/latency",
		Summary: "Get event latency", Description: "Returns latency metrics for event processing.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleEventLatency)

	RegisterTypedOp(api, OpMeta{
		ID: "get-cost-forecast", Method: http.MethodGet, Path: "/v1/analytics/costs/forecast",
		Summary: "Get cost forecast", Description: "Returns projected costs based on historical usage trends.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleCostForecast)

	RegisterTypedOp(api, OpMeta{
		ID: "get-cost-by-trigger", Method: http.MethodGet, Path: "/v1/analytics/costs/by-trigger",
		Summary: "Get cost by trigger type", Description: "Returns cost breakdown grouped by trigger type.",
		Tags: []string{"Analytics"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleCostByTrigger)

	// -- RBAC: Roles --
	RegisterTypedOp(api, OpMeta{
		ID: "create-role", Method: http.MethodPost, Path: "/v1/roles",
		Summary: "Create a role", Description: "Creates a new custom role with the specified permissions.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateRole)

	RegisterTypedOp(api, OpMeta{
		ID: "list-roles", Method: http.MethodGet, Path: "/v1/roles",
		Summary: "List roles", Description: "Returns all roles defined in the current project.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListRoles)

	RegisterTypedOp(api, OpMeta{
		ID: "get-role", Method: http.MethodGet, Path: "/v1/roles/{roleID}",
		Summary: "Get a role", Description: "Returns details of a specific role including its permissions.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetRole)

	RegisterTypedOp(api, OpMeta{
		ID: "update-role", Method: http.MethodPatch, Path: "/v1/roles/{roleID}",
		Summary: "Update a role", Description: "Updates an existing role's name or permissions.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleUpdateRole)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-role", Method: http.MethodDelete, Path: "/v1/roles/{roleID}",
		Summary: "Delete a role", Description: "Permanently deletes a custom role. Members with this role lose its permissions.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleDeleteRole)

	// -- RBAC: Members --
	RegisterTypedOp(api, OpMeta{
		ID: "assign-member", Method: http.MethodPost, Path: "/v1/members",
		Summary: "Assign a member to a role", Description: "Assigns a user to a role in the current project.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleAssignMember)

	RegisterTypedOp(api, OpMeta{
		ID: "bulk-assign-members", Method: http.MethodPost, Path: "/v1/members/bulk",
		Summary: "Bulk assign members", Description: "Assigns multiple users to roles in a single operation.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleBulkAssignMembers)

	RegisterTypedOp(api, OpMeta{
		ID: "list-members", Method: http.MethodGet, Path: "/v1/members",
		Summary: "List members", Description: "Returns all members and their roles in the current project.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListMembers)

	RegisterTypedOp(api, OpMeta{
		ID: "remove-member", Method: http.MethodDelete, Path: "/v1/members/{userID}",
		Summary: "Remove a member", Description: "Removes a user's role assignment from the current project.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleRemoveMember)

	// -- RBAC: Seed Roles --
	RegisterTypedOp(api, OpMeta{
		ID: "seed-system-roles", Method: http.MethodPost, Path: "/v1/seed-roles",
		Summary: "Seed system roles", Description: "Creates the default system roles (admin, editor, viewer) if they do not exist.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleSeedSystemRoles)

	// -- Audit --
	RegisterTypedOp(api, OpMeta{
		ID: "list-audit-events", Method: http.MethodGet, Path: "/v1/audit-events",
		Summary: "List audit events", Description: "Returns a paginated list of audit events for the current project.",
		Tags: []string{"Audit"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListAuditEvents)

	RegisterTypedOp(api, OpMeta{
		ID: "export-audit-events", Method: http.MethodGet, Path: "/v1/audit-events/export",
		Summary: "Export audit events", Description: "Exports audit events as CSV or JSON for compliance and reporting.",
		Tags: []string{"Audit"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleExportAuditEvents)

	RegisterTypedOp(api, OpMeta{
		ID: "get-audit-event", Method: http.MethodGet, Path: "/v1/audit-events/{id}",
		Summary: "Get an audit event", Description: "Returns a single audit event scoped to the current project. The read itself is audited.",
		Tags: []string{"Audit"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetAuditEvent)

	// -- Data Export --
	RegisterTypedOp(api, OpMeta{
		ID: "export-jobs", Method: http.MethodGet, Path: "/v1/export/jobs",
		Summary: "Export jobs", Description: "Streams all job definitions for the current project as JSON or NDJSON.",
		Tags: []string{"Export"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleExportJobs)

	RegisterTypedOp(api, OpMeta{
		ID: "export-runs", Method: http.MethodGet, Path: "/v1/export/runs",
		Summary: "Export runs", Description: "Streams run history for the current project within a time window as JSON, NDJSON, or CSV.",
		Tags: []string{"Export"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleExportRuns)

	RegisterTypedOp(api, OpMeta{
		ID: "export-workflows", Method: http.MethodGet, Path: "/v1/export/workflows",
		Summary: "Export workflows", Description: "Streams all workflow definitions for the current project as JSON or NDJSON.",
		Tags: []string{"Export"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleExportWorkflows)

	RegisterTypedOp(api, OpMeta{
		ID: "verify-audit-chain", Method: http.MethodGet, Path: "/v1/audit-events/verify",
		Summary: "Verify audit chain", Description: "Verifies the integrity of the audit event hash chain.",
		Tags: []string{"Audit"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleVerifyAuditChain)

	// -- Audit deadletter (admin) --
	RegisterTypedOp(api, OpMeta{
		ID: "list-audit-deadletter", Method: http.MethodGet, Path: "/v1/audit/deadletter",
		Summary: "List audit deadletter entries", Description: "Returns a paginated list of audit events that failed to persist to the chain and are awaiting reclamation. Admin-only.",
		Tags: []string{"Audit"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 500},
	}, s.handleListDeadletter)

	RegisterTypedOp(api, OpMeta{
		ID: "replay-audit-deadletter", Method: http.MethodPost, Path: "/v1/audit/deadletter/{id}/replay",
		Summary: "Replay an audit deadletter entry", Description: "Moves a deadletter entry into the primary audit chain and removes it from the DLQ on success. Admin-only.",
		Tags: []string{"Audit"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleReplayDeadletter)

	RegisterTypedOp(api, OpMeta{
		ID: "drop-audit-deadletter", Method: http.MethodDelete, Path: "/v1/audit/deadletter/{id}",
		Summary: "Drop an audit deadletter entry", Description: "Permanently deletes a deadletter entry, accepting data loss. Admin-only. The drop itself is audited.",
		Tags: []string{"Audit"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleDropDeadletter)

	RegisterTypedOp(api, OpMeta{
		ID: "update-audit-export-cap", Method: http.MethodPut, Path: "/v1/projects/{id}/quotas/audit-export-cap",
		Summary:     "Update audit export row cap",
		Description: "Sets the per-project row cap applied to audit export streams. 0 re-inherits the server default. Admin-only, audited.",
		Tags:        []string{"Audit"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 500},
	}, s.handleUpdateAuditExportCap)

	RegisterTypedOp(api, OpMeta{
		ID: "get-audit-retention", Method: http.MethodGet, Path: "/v1/projects/{id}/audit/retention",
		Summary:     "Get audit retention override",
		Description: "Returns the per-project audit retention window (days). Reports whether the value is inherited from the server default or explicitly overridden (including the explicit 0 = disable-trim case). Admin-only.",
		Tags:        []string{"Audit"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 500},
	}, s.handleGetAuditRetention)

	RegisterTypedOp(api, OpMeta{
		ID: "set-audit-retention", Method: http.MethodPut, Path: "/v1/projects/{id}/audit/retention",
		Summary:     "Update audit retention override",
		Description: "Sets the per-project audit retention window (days). 0 disables retention trimming for the project; negative values are rejected. Admin-only, audited.",
		Tags:        []string{"Audit"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 500},
	}, s.handleSetAuditRetention)

	RegisterTypedOp(api, OpMeta{
		ID: "rotate-audit-signing-key", Method: http.MethodPost, Path: "/v1/projects/{id}/audit/rotate-key",
		Summary:     "Rotate the audit signing key",
		Description: "Rotates the per-project HMAC signing key for the audit chain. Stores the new per-epoch key encrypted and emits an is_anchor=TRUE audit.key_rotated event under the new key. Admin-only.",
		Tags:        []string{"Audit"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 500},
	}, s.handleRotateAuditSigningKey)

	// -- RBAC: Resource Policies --
	RegisterTypedOp(api, OpMeta{
		ID: "create-resource-policy", Method: http.MethodPost, Path: "/v1/resource-policies",
		Summary: "Create a resource policy", Description: "Creates a policy restricting access to specific resources by role.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateResourcePolicy)

	RegisterTypedOp(api, OpMeta{
		ID: "list-resource-policies", Method: http.MethodGet, Path: "/v1/resource-policies",
		Summary: "List resource policies", Description: "Returns all resource policies in the current project.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListResourcePolicies)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-resource-policy", Method: http.MethodDelete, Path: "/v1/resource-policies/{policyID}",
		Summary: "Delete a resource policy", Description: "Permanently deletes a resource policy.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleDeleteResourcePolicy)

	// -- RBAC: Tag Policies --
	RegisterTypedOp(api, OpMeta{
		ID: "create-tag-policy", Method: http.MethodPost, Path: "/v1/tag-policies",
		Summary: "Create a tag policy", Description: "Creates a policy restricting access based on resource tags.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateTagPolicy)

	RegisterTypedOp(api, OpMeta{
		ID: "list-tag-policies", Method: http.MethodGet, Path: "/v1/tag-policies",
		Summary: "List tag policies", Description: "Returns all tag policies in the current project.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListTagPolicies)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-tag-policy", Method: http.MethodDelete, Path: "/v1/tag-policies/{policyID}",
		Summary: "Delete a tag policy", Description: "Permanently deletes a tag policy.",
		Tags: []string{"RBAC"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleDeleteTagPolicy)

	// -- Workflow Policies --
	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-policy", Method: http.MethodGet, Path: "/v1/workflow-policies/{projectID}",
		Summary: "Get workflow policy", Description: "Returns the workflow execution policy for a project.",
		Tags: []string{"Workflow Policies"}, Security: bearerSecurity, Errors: []int{401, 403, 404, 500},
	}, s.handleGetWorkflowPolicy)

	RegisterTypedOp(api, OpMeta{
		ID: "upsert-workflow-policy", Method: http.MethodPut, Path: "/v1/workflow-policies/{projectID}",
		Summary: "Create or update workflow policy", Description: "Creates or updates the workflow execution policy for a project.",
		Tags: []string{"Workflow Policies"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleUpsertWorkflowPolicy)

	// -- Workflows --
	RegisterTypedOp(api, OpMeta{
		ID: "create-workflow", Method: http.MethodPost, Path: "/v1/workflows",
		Summary: "Create a workflow", Description: "Creates a new workflow with step definitions and trigger configuration.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateWorkflow)

	RegisterTypedOp(api, OpMeta{
		ID: "list-workflows", Method: http.MethodGet, Path: "/v1/workflows",
		Summary: "List workflows", Description: "Returns a paginated list of workflows in the current project.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListWorkflows)

	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow", Method: http.MethodGet, Path: "/v1/workflows/{workflowID}",
		Summary: "Get a workflow", Description: "Returns details of a specific workflow including its step definitions.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetWorkflow)

	RegisterTypedOp(api, OpMeta{
		ID: "update-workflow", Method: http.MethodPatch, Path: "/v1/workflows/{workflowID}",
		Summary: "Update a workflow", Description: "Updates an existing workflow's configuration and step definitions. Set breaking_change to true to acknowledge a breaking update; an audit event is emitted when active runs exist on the previous version.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleUpdateWorkflow)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-workflow", Method: http.MethodDelete, Path: "/v1/workflows/{workflowID}",
		Summary: "Delete a workflow", Description: "Permanently deletes a workflow and all its associated runs.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleDeleteWorkflow)

	RegisterTypedOp(api, OpMeta{
		ID: "dry-run-workflow", Method: http.MethodPost, Path: "/v1/workflows/{workflowID}/dry-run",
		Summary: "Dry-run a workflow", Description: "Validates a workflow execution without creating actual runs.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleDryRunWorkflow)

	RegisterTypedOp(api, OpMeta{
		ID: "plan-workflow", Method: http.MethodPost, Path: "/v1/workflows/{workflowID}/plan",
		Summary: "Plan a workflow execution", Description: "Generates an execution plan showing which steps will run and in what order.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleWorkflowPlan)

	RegisterTypedOp(api, OpMeta{
		ID: "simulate-workflow", Method: http.MethodPost, Path: "/v1/workflows/{workflowID}/simulate",
		Summary: "Simulate a workflow execution", Description: "Simulates a workflow run with mock data to preview step outcomes.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleSimulateWorkflow)

	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-graph", Method: http.MethodGet, Path: "/v1/workflows/{workflowID}/graph",
		Summary: "Get workflow graph", Description: "Returns the DAG representation of the workflow's step dependencies.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleWorkflowGraph)

	RegisterTypedOp(api, OpMeta{
		ID: "trigger-workflow", Method: http.MethodPost, Path: "/v1/workflows/{workflowID}/trigger",
		Summary: "Trigger a workflow", Description: "Triggers a new execution of the workflow.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 429, 500, 503},
	}, s.handleTriggerWorkflow)

	RegisterTypedOp(api, OpMeta{
		ID: "clone-workflow", Method: http.MethodPost, Path: "/v1/workflows/{workflowID}/clone",
		Summary: "Clone a workflow", Description: "Creates a copy of an existing workflow with a new name.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleCloneWorkflow)

	RegisterTypedOp(api, OpMeta{
		ID: "list-workflow-runs-by-workflow", Method: http.MethodGet, Path: "/v1/workflows/{workflowID}/runs",
		Summary: "List runs for a workflow", Description: "Returns a paginated list of runs for a specific workflow.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListWorkflowRuns)

	RegisterTypedOp(api, OpMeta{
		ID: "list-workflow-versions", Method: http.MethodGet, Path: "/v1/workflows/{workflowID}/versions",
		Summary: "List workflow versions", Description: "Returns all versions of a workflow definition.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListWorkflowVersions)

	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-version", Method: http.MethodGet, Path: "/v1/workflows/{workflowID}/versions/{versionID}",
		Summary: "Get a workflow version", Description: "Returns details of a specific workflow version.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetWorkflowVersion)

	RegisterTypedOp(api, OpMeta{
		ID: "list-workflow-version-steps", Method: http.MethodGet, Path: "/v1/workflows/{workflowID}/versions/{versionID}/steps",
		Summary: "List workflow version steps", Description: "Returns the step definitions for a specific workflow version.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleListWorkflowVersionSteps)

	RegisterTypedOp(api, OpMeta{
		ID: "diff-workflow-versions", Method: http.MethodGet, Path: "/v1/workflows/{workflowID}/versions/{fromVersionID}/diff/{toVersionID}",
		Summary: "Diff workflow versions", Description: "Returns the differences between two workflow versions.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleWorkflowVersionDiff)

	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-version-impact", Method: http.MethodGet, Path: "/v1/workflows/{workflowID}/versions/{versionID}/impact",
		Summary: "Get workflow version impact", Description: "Returns the impact analysis for a specific workflow version.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleWorkflowVersionImpact)

	RegisterTypedOp(api, OpMeta{
		ID: "get-active-versions", Method: http.MethodGet, Path: "/v1/workflows/{workflowID}/active-versions",
		Summary: "Get active workflow versions", Description: "Returns the currently active versions for a workflow, including traffic splits.",
		Tags: []string{"Workflows"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleGetActiveVersions)

	// -- Deployments --
	RegisterTypedOp(api, OpMeta{
		ID: "create-deployment", Method: http.MethodPost, Path: "/v1/deployments",
		Summary: "Create a deployment version", Description: "Creates a new deployment version for a workflow.",
		Tags: []string{"Deployments"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateDeploymentVersion)

	RegisterTypedOp(api, OpMeta{
		ID: "list-deployments", Method: http.MethodGet, Path: "/v1/deployments",
		Summary: "List deployment versions", Description: "Returns a paginated list of deployment versions.",
		Tags: []string{"Deployments"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListDeploymentVersions)

	RegisterTypedOp(api, OpMeta{
		ID: "finalize-deployment", Method: http.MethodPost, Path: "/v1/deployments/{deploymentID}/finalize",
		Summary: "Finalize a deployment version", Description: "Finalizes a deployment version, locking its configuration.",
		Tags: []string{"Deployments"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleFinalizeDeploymentVersion)

	RegisterTypedOp(api, OpMeta{
		ID: "promote-deployment", Method: http.MethodPost, Path: "/v1/deployments/{deploymentID}/promote",
		Summary: "Promote a deployment version", Description: "Promotes a deployment version to active, routing traffic to it.",
		Tags: []string{"Deployments"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handlePromoteDeploymentVersion)

	RegisterTypedOp(api, OpMeta{
		ID: "rollback-deployment", Method: http.MethodPost, Path: "/v1/deployments/{deploymentID}/rollback",
		Summary: "Rollback a deployment version", Description: "Rolls back to the previous deployment version.",
		Tags: []string{"Deployments"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleRollbackDeploymentVersion)

	// -- Event Sources --
	RegisterTypedOp(api, OpMeta{
		ID: "list-event-sources", Method: http.MethodGet, Path: "/v1/event-sources",
		Summary: "List event sources", Description: "Returns all event sources configured in the current project.",
		Tags: []string{"Event Sources"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListEventSources)

	RegisterTypedOp(api, OpMeta{
		ID: "create-event-source", Method: http.MethodPost, Path: "/v1/event-sources",
		Summary: "Create an event source", Description: "Creates a new event source for receiving external events.",
		Tags: []string{"Event Sources"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleCreateEventSource)

	RegisterTypedOp(api, OpMeta{
		ID: "get-event-source", Method: http.MethodGet, Path: "/v1/event-sources/{sourceID}",
		Summary: "Get an event source", Description: "Returns details of a specific event source.",
		Tags: []string{"Event Sources"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetEventSource)

	RegisterTypedOp(api, OpMeta{
		ID: "update-event-source", Method: http.MethodPatch, Path: "/v1/event-sources/{sourceID}",
		Summary: "Update an event source", Description: "Updates an existing event source's configuration.",
		Tags: []string{"Event Sources"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleUpdateEventSource)

	RegisterTypedOp(api, OpMeta{
		ID: "delete-event-source", Method: http.MethodDelete, Path: "/v1/event-sources/{sourceID}",
		Summary: "Delete an event source", Description: "Permanently deletes an event source and its subscriptions.",
		Tags: []string{"Event Sources"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleDeleteEventSource)

	RegisterTypedOp(api, OpMeta{
		ID: "list-event-source-subscriptions", Method: http.MethodGet, Path: "/v1/event-sources/{sourceID}/subscriptions",
		Summary: "List event source subscriptions", Description: "Returns all subscriptions for a specific event source.",
		Tags: []string{"Event Sources"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListEventSourceSubscriptions)

	RegisterTypedOp(api, OpMeta{
		ID: "subscribe-to-event-source", Method: http.MethodPost, Path: "/v1/event-sources/{sourceID}/subscribe",
		Summary: "Subscribe to an event source", Description: "Creates a subscription linking a job or workflow to an event source.",
		Tags: []string{"Event Sources"}, Security: bearerSecurity, Errors: []int{400, 401, 409, 429, 500},
	}, s.handleSubscribeToEventSource)

	// Registered manually (not via RegisterTypedOp) because RegisterTypedOp
	// with the handler's *struct{} output type drops the sourceID path param.
	huma.Register(api, huma.Operation{
		OperationID: "delete-event-subscription",
		Method:      http.MethodDelete,
		Path:        "/v1/event-sources/{sourceID}/subscriptions/{subID}",
		Summary:     "Delete an event subscription",
		Description: "Removes a subscription from an event source.",
		Tags:        []string{"Event Sources"},
		Security:    bearerSecurity,
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		SourceID string `path:"sourceID" doc:"Event source ID" example:"src_01HX8BQNP4"`
		SubID    string `path:"subID" doc:"Event subscription ID" example:"sub_01HX8BQNP4"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	// -- Events --
	RegisterTypedOp(api, OpMeta{
		ID: "dispatch-event", Method: http.MethodPost, Path: "/v1/events/dispatch",
		Summary: "Dispatch an event", Description: "Dispatches an event that triggers matching subscriptions.",
		Tags: []string{"Events"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 429, 500},
	}, s.handleDispatchEvent)

	RegisterTypedOp(api, OpMeta{
		ID: "list-event-triggers", Method: http.MethodGet, Path: "/v1/events",
		Summary: "List event triggers", Description: "Returns a paginated list of event triggers in the current project.",
		Tags: []string{"Events"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListEventTriggers)

	RegisterTypedOp(api, OpMeta{
		ID: "get-event-trigger-stats", Method: http.MethodGet, Path: "/v1/events/stats",
		Summary: "Get event trigger statistics", Description: "Returns aggregate statistics about event triggers.",
		Tags: []string{"Events"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetEventTriggerStats)

	RegisterTypedOp(api, OpMeta{
		ID: "purge-event-triggers", Method: http.MethodPost, Path: "/v1/events/purge",
		Summary: "Purge event triggers", Description: "Purges completed or expired event triggers.",
		Tags: []string{"Events"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handlePurgeEventTriggers)

	RegisterTypedOp(api, OpMeta{
		ID: "send-event-by-prefix", Method: http.MethodPost, Path: "/v1/events/prefix/{prefix}/send",
		Summary: "Send event by prefix", Description: "Sends an event to all triggers matching the given key prefix.",
		Tags: []string{"Events"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 429, 500},
	}, s.handleSendEventByPrefix)

	RegisterTypedOp(api, OpMeta{
		ID: "get-event-trigger", Method: http.MethodGet, Path: "/v1/events/{eventKey}",
		Summary: "Get an event trigger", Description: "Returns details of a specific event trigger by its key.",
		Tags: []string{"Events"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetEventTrigger)

	RegisterTypedOp(api, OpMeta{
		ID: "cancel-event-trigger", Method: http.MethodDelete, Path: "/v1/events/{eventKey}",
		Summary: "Cancel an event trigger", Description: "Cancels a waiting event trigger.",
		Tags: []string{"Events"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 409, 500},
	}, s.handleCancelEventTrigger)

	RegisterTypedOp(api, OpMeta{
		ID: "send-event", Method: http.MethodPost, Path: "/v1/events/{eventKey}/send",
		Summary: "Send an event", Description: "Sends a payload to an event trigger, resolving it.",
		Tags: []string{"Events"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 409, 429, 500},
	}, s.handleSendEvent)

	// -- Workflow Runs --
	RegisterTypedOp(api, OpMeta{
		ID: "list-workflow-runs", Method: http.MethodGet, Path: "/v1/workflow-runs",
		Summary: "List workflow runs", Description: "Returns a paginated list of workflow runs in the current project.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListWorkflowRunsByProject)

	RegisterTypedOp(api, OpMeta{
		ID: "bulk-cancel-workflow-runs", Method: http.MethodPost, Path: "/v1/workflow-runs/bulk-cancel",
		Summary: "Bulk cancel workflow runs", Description: "Cancels multiple workflow runs matching the provided filters.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleBulkCancelWorkflowRuns)

	RegisterTypedOp(api, OpMeta{
		ID: "bulk-replay-workflow-runs", Method: http.MethodPost, Path: "/v1/workflow-runs/bulk-replay",
		Summary: "Bulk replay workflow runs", Description: "Replays multiple workflow runs matching the provided filters.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500, 503},
	}, s.handleBulkReplayWorkflowRuns)

	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-run", Method: http.MethodGet, Path: "/v1/workflow-runs/{workflowRunID}",
		Summary: "Get a workflow run", Description: "Returns details of a specific workflow run including step statuses.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleGetWorkflowRun)

	RegisterTypedOp(api, OpMeta{
		ID: "cancel-workflow-run", Method: http.MethodDelete, Path: "/v1/workflow-runs/{workflowRunID}",
		Summary: "Cancel a workflow run", Description: "Cancels an active workflow run and all its pending steps.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleCancelWorkflowRun)

	RegisterTypedOp(api, OpMeta{
		ID: "pause-workflow-run", Method: http.MethodPost, Path: "/v1/workflow-runs/{workflowRunID}/pause",
		Summary: "Pause a workflow run", Description: "Pauses an active workflow run, preventing further steps from executing.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handlePauseWorkflowRun)

	RegisterTypedOp(api, OpMeta{
		ID: "resume-workflow-run", Method: http.MethodPost, Path: "/v1/workflow-runs/{workflowRunID}/resume",
		Summary: "Resume a workflow run", Description: "Resumes a paused workflow run.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500, 503},
	}, s.handleResumeWorkflowRun)

	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-run-labels", Method: http.MethodGet, Path: "/v1/workflow-runs/{workflowRunID}/labels",
		Summary: "Get workflow run labels", Description: "Returns the labels attached to a workflow run.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetWorkflowRunLabels)

	RegisterTypedOp(api, OpMeta{
		ID: "list-workflow-step-runs", Method: http.MethodGet, Path: "/v1/workflow-runs/{workflowRunID}/steps",
		Summary: "List workflow step runs", Description: "Returns all step runs for a specific workflow run.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleListWorkflowStepRuns)

	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-run-graph", Method: http.MethodGet, Path: "/v1/workflow-runs/{workflowRunID}/graph",
		Summary: "Get workflow run graph", Description: "Returns the execution graph for a workflow run with step statuses.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetWorkflowRunGraph)

	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-run-explain", Method: http.MethodGet, Path: "/v1/workflow-runs/{workflowRunID}/explain",
		Summary: "Explain workflow run execution", Description: "Returns a human-readable explanation of workflow run execution decisions.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetWorkflowRunExplain)

	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-run-timeline", Method: http.MethodGet, Path: "/v1/workflow-runs/{workflowRunID}/timeline",
		Summary: "Get workflow run timeline", Description: "Returns a chronological timeline of events for a workflow run.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleGetWorkflowRunTimeline)

	// -- Workflow Steps --
	RegisterTypedOp(api, OpMeta{
		ID: "approve-workflow-step", Method: http.MethodPost, Path: "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/approve",
		Summary: "Approve a workflow step", Description: "Approves a workflow step that is waiting for manual approval.",
		Tags: []string{"Workflow Steps"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500, 503},
	}, s.handleApproveWorkflowStep)

	RegisterTypedOp(api, OpMeta{
		ID: "skip-workflow-step", Method: http.MethodPost, Path: "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/skip",
		Summary: "Skip a workflow step", Description: "Skips a pending workflow step and continues execution.",
		Tags: []string{"Workflow Steps"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500, 503},
	}, s.handleSkipWorkflowStep)

	RegisterTypedOp(api, OpMeta{
		ID: "force-complete-workflow-step", Method: http.MethodPost, Path: "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/force-complete",
		Summary: "Force-complete a workflow step", Description: "Forces a workflow step to complete regardless of its current state.",
		Tags: []string{"Workflow Steps"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500, 503},
	}, s.handleForceCompleteWorkflowStep)

	RegisterTypedOp(api, OpMeta{
		ID: "retry-workflow-step", Method: http.MethodPost, Path: "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/retry",
		Summary: "Retry a workflow step", Description: "Retries a failed workflow step.",
		Tags: []string{"Workflow Steps"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500, 503},
	}, s.handleRetryWorkflowStep)

	RegisterTypedOp(api, OpMeta{
		ID: "replay-workflow-subtree", Method: http.MethodPost, Path: "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/replay-subtree",
		Summary: "Replay a workflow subtree", Description: "Replays a workflow step and all of its downstream dependent steps.",
		Tags: []string{"Workflow Steps"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500, 503},
	}, s.handleReplayWorkflowSubtree)

	RegisterTypedOp(api, OpMeta{
		ID: "retry-workflow-run", Method: http.MethodPost, Path: "/v1/workflow-runs/{workflowRunID}/retry",
		Summary: "Retry a workflow run", Description: "Retries a failed workflow run from the beginning.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500, 503},
	}, s.handleRetryWorkflowRun)

	// -- Compensation --
	RegisterTypedOp(api, OpMeta{
		ID: "compensate-workflow-run", Method: http.MethodPost, Path: "/v1/workflow-runs/{workflowRunID}/compensate",
		Summary: "Compensate a failed workflow run", Description: "Triggers compensation for previously completed steps in reverse topological order. Only valid for failed or timed_out runs.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleCompensateWorkflowRun)

	RegisterTypedOp(api, OpMeta{
		ID: "get-compensation-plan", Method: http.MethodGet, Path: "/v1/workflow-runs/{workflowRunID}/compensation-plan",
		Summary: "Get compensation plan", Description: "Returns the compensation plan for a workflow run without executing it. Shows which steps would be compensated and in what order.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleGetCompensationPlan)

	// -- Debug --
	RegisterTypedOp(api, OpMeta{
		ID: "get-workflow-run-debug", Method: http.MethodGet, Path: "/v1/workflow-runs/{workflowRunID}/debug",
		Summary: "Get workflow run debug view", Description: "Returns a full debug timeline with per-step status, timing, cost, input/output, and data flow between steps.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleGetWorkflowRunDebug)

	RegisterTypedOp(api, OpMeta{
		ID: "compare-workflow-runs", Method: http.MethodGet, Path: "/v1/workflow-runs/{workflowRunID}/compare/{otherRunID}",
		Summary: "Compare two workflow runs", Description: "Returns a diff between two workflow runs highlighting status changes, duration differences, and steps present in only one run.",
		Tags: []string{"Workflow Runs"}, Security: bearerSecurity, Errors: []int{401, 404, 500},
	}, s.handleCompareWorkflowRuns)

	// -- Canary Deployments --
	RegisterTypedOp(api, OpMeta{
		ID: "create-canary-deployment", Method: http.MethodPost, Path: "/v1/canary-deployments",
		Summary: "Create canary deployment", Description: "Creates a canary deployment to gradually shift traffic from one workflow version to another.",
		Tags: []string{"Canary Deployments"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 409, 500},
	}, s.handleCreateCanaryDeployment)

	RegisterTypedOp(api, OpMeta{
		ID: "update-canary-deployment", Method: http.MethodPatch, Path: "/v1/workflows/{workflowID}/canary",
		Summary: "Update canary traffic", Description: "Adjusts the traffic percentage for an active canary deployment.",
		Tags: []string{"Canary Deployments"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 500},
	}, s.handleUpdateCanaryDeployment)

	RegisterTypedOp(api, OpMeta{
		ID: "rollback-canary-deployment", Method: http.MethodPost, Path: "/v1/workflows/{workflowID}/canary/rollback",
		Summary: "Rollback canary deployment", Description: "Immediately rolls back a canary deployment to 0% target traffic.",
		Tags: []string{"Canary Deployments"}, Security: bearerSecurity, Errors: []int{401, 403, 404, 500},
	}, s.handleRollbackCanaryDeployment)

	RegisterTypedOp(api, OpMeta{
		ID: "get-canary-status", Method: http.MethodGet, Path: "/v1/workflows/{workflowID}/canary",
		Summary: "Get canary deployment status", Description: "Returns the current canary deployment status with version comparison metrics.",
		Tags: []string{"Canary Deployments"}, Security: bearerSecurity, Errors: []int{401, 403, 404, 500},
	}, s.handleGetCanaryStatus)

	// -- SDK --
	RegisterTypedOp(api, OpMeta{
		ID: "sdk-get-payload", Method: http.MethodGet, Path: "/sdk/v1/runs/{runID}/payload",
		Summary: "Get run payload", Description: "Returns the payload for a run so the SDK can begin execution.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleSDKGetPayload)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-log", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/log",
		Summary: "Send a log entry", Description: "Sends a structured log entry from the running job to Strait.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleSDKLog)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-progress", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/progress",
		Summary: "Report execution progress", Description: "Reports execution progress as a percentage for monitoring.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleSDKProgress)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-annotate", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/annotate",
		Summary: "Annotate a run", Description: "Attaches metadata annotations to a run for search and filtering.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleSDKAnnotate)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-heartbeat", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/heartbeat",
		Summary: "Send a heartbeat", Description: "Sends a heartbeat to indicate the run is still actively executing.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleSDKHeartbeat)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-checkpoint", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/checkpoint",
		Summary: "Save a checkpoint", Description: "Saves a checkpoint so the run can resume from this point on retry.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleSDKCheckpoint)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-output", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/output",
		Summary: "Record an output", Description: "Records a structured output produced by the run.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleSDKOutput)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-complete", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/complete",
		Summary: "Mark run as complete", Description: "Marks the run as successfully completed with optional result data.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 413, 500},
	}, s.handleSDKComplete)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-fail", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/fail",
		Summary: "Mark run as failed", Description: "Marks the run as failed with an error message and optional details.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleSDKFail)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-spawn", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/spawn",
		Summary: "Spawn a child run", Description: "Spawns a child run from within the current run for fan-out patterns.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 403, 404, 409, 500},
	}, s.handleSDKSpawn)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-continue", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/continue",
		Summary: "Continue execution", Description: "Signals that the run should continue with a new execution step.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 500},
	}, s.handleSDKContinue)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-wait-for-event", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/wait-for-event",
		Summary: "Wait for an external event", Description: "Pauses the run until an external event with the specified key is received.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 409, 429, 500},
	}, s.handleSDKWaitForEvent)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-set-state", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/state",
		Summary: "Set run state", Description: "Sets a key-value pair in the run's state store.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleSDKSetState)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-list-state", Method: http.MethodGet, Path: "/sdk/v1/runs/{runID}/state",
		Summary: "List run state", Description: "Returns all key-value pairs in the run's state store.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleSDKListState)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-get-state", Method: http.MethodGet, Path: "/sdk/v1/runs/{runID}/state/{key}",
		Summary: "Get a state value", Description: "Returns a specific value from the run's state store by key.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleSDKGetState)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-delete-state", Method: http.MethodDelete, Path: "/sdk/v1/runs/{runID}/state/{key}",
		Summary: "Delete a state value", Description: "Removes a key-value pair from the run's state store.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleSDKDeleteState)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-stream-chunk", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/stream",
		Summary: "Send a stream chunk", Description: "Sends a streaming chunk for real-time output from runs.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleSDKStreamChunk)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-resources", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/resources",
		Summary: "Report resource utilization", Description: "Reports CPU, memory, and other resource utilization during execution.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleSDKResources)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-resource-snapshot", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/resource-snapshot",
		Summary: "Save a resource snapshot", Description: "Saves a point-in-time snapshot of resource utilization.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 500},
	}, s.handleSDKResourceSnapshot)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-set-memory", Method: http.MethodPost, Path: "/sdk/v1/runs/{runID}/memory/{key}",
		Summary: "Set a memory value", Description: "Stores a value in persistent memory that survives across run attempts.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleSDKSetMemory)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-get-memory", Method: http.MethodGet, Path: "/sdk/v1/runs/{runID}/memory/{key}",
		Summary: "Get a memory value", Description: "Retrieves a value from persistent memory by key.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleSDKGetMemory)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-list-memory", Method: http.MethodGet, Path: "/sdk/v1/runs/{runID}/memory",
		Summary: "List memory entries", Description: "Returns all key-value pairs in persistent memory for the run's job.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleSDKListMemory)

	RegisterTypedOp(api, OpMeta{
		ID: "sdk-delete-memory", Method: http.MethodDelete, Path: "/sdk/v1/runs/{runID}/memory/{key}",
		Summary: "Delete a memory value", Description: "Removes a key-value pair from persistent memory.",
		Tags: []string{"SDK"}, Security: bearerSecurity, Errors: []int{400, 401, 404, 500},
	}, s.handleSDKDeleteMemory)
}
