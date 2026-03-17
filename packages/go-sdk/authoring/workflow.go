package authoring

import (
	"context"
	"encoding/json"
	"fmt"
)

// WorkflowOptions configures a workflow definition.
type WorkflowOptions[TPayload any] struct {
	Name      string
	Slug      string
	Steps     []Step
	ProjectID string

	Description       string
	Tags              map[string]string
	EnvironmentID     string
	MaxConcurrentRuns *int
	MaxParallelSteps  *int
	TimeoutSecs       *int
	MaxAttempts       *int
	RetryStrategy     string
	Cron              string
	Timezone          string
	WebhookURL        string
	WebhookSecret     string

	// Run is the handler executed when the workflow runs. Stored but not invoked by the SDK.
	Run func(payload TPayload, ctx RunContext) (any, error)
	// OnSuccess is called after a successful run. Stored but not invoked by the SDK.
	OnSuccess func(payload TPayload, output any, ctx RunContext) error
	// OnFailure is called when a run fails. Stored but not invoked by the SDK.
	OnFailure func(payload TPayload, err error, ctx RunContext) error
}

// WorkflowDefinition is a defined workflow with registration and triggering methods.
type WorkflowDefinition[TPayload any] struct {
	Kind string
	Slug string
	opts WorkflowOptions[TPayload]

	Run       func(payload TPayload, ctx RunContext) (any, error)
	OnSuccess func(payload TPayload, output any, ctx RunContext) error
	OnFailure func(payload TPayload, err error, ctx RunContext) error

	lastRegisteredWorkflowID string
}

// TriggerWorkflowInput is the input for triggering a workflow run.
type TriggerWorkflowInput[TPayload any] struct {
	WorkflowID     string
	Payload        TPayload
	IdempotencyKey string
	Priority       *int
	DryRun         *bool
	Metadata       map[string]string
	StepOverrides  map[string]any
}

// WorkflowDSLClient abstracts the API operations needed by workflow definitions.
type WorkflowDSLClient interface {
	CreateWorkflow(ctx context.Context, body map[string]any) (map[string]any, error)
	TriggerWorkflow(ctx context.Context, workflowID string, body map[string]any) (map[string]any, error)
	GetRun(ctx context.Context, runID string) (map[string]any, error)
}

// DefineWorkflow creates a new workflow definition.
func DefineWorkflow[TPayload any](opts WorkflowOptions[TPayload]) *WorkflowDefinition[TPayload] {
	return &WorkflowDefinition[TPayload]{
		Kind:      "workflow",
		Slug:      opts.Slug,
		opts:      opts,
		Run:       opts.Run,
		OnSuccess: opts.OnSuccess,
		OnFailure: opts.OnFailure,
	}
}

// ToRegistrationBody builds the snake_case API registration body.
// It validates the DAG and converts steps to API format.
func (w *WorkflowDefinition[TPayload]) ToRegistrationBody(projectID string) (map[string]any, error) {
	pid := w.opts.ProjectID
	if projectID != "" {
		pid = projectID
	}
	if pid == "" {
		return nil, fmt.Errorf("defineWorkflow(%s) requires projectId", w.Slug)
	}

	// Validate DAG
	if len(w.opts.Steps) > 0 {
		if _, err := ValidateDag(w.opts.Steps); err != nil {
			return nil, err
		}
	}

	// Convert steps to API format
	apiSteps := make([]map[string]any, len(w.opts.Steps))
	for i, s := range w.opts.Steps {
		apiSteps[i] = StepToAPI(s)
	}

	body := map[string]any{
		"project_id": pid,
		"name":       w.opts.Name,
		"slug":       w.opts.Slug,
		"steps":      apiSteps,
	}

	setOptStr(body, "description", w.opts.Description)
	setOptMap(body, "tags", w.opts.Tags)
	setOptStr(body, "environment_id", w.opts.EnvironmentID)
	setOptInt(body, "max_concurrent_runs", w.opts.MaxConcurrentRuns)
	setOptInt(body, "max_parallel_steps", w.opts.MaxParallelSteps)
	setOptInt(body, "timeout_secs", w.opts.TimeoutSecs)
	setOptInt(body, "max_attempts", w.opts.MaxAttempts)
	setOptStr(body, "retry_strategy", w.opts.RetryStrategy)
	setOptStr(body, "cron", w.opts.Cron)
	setOptStr(body, "timezone", w.opts.Timezone)
	setOptStr(body, "webhook_url", w.opts.WebhookURL)
	setOptStr(body, "webhook_secret", w.opts.WebhookSecret)

	return body, nil
}

// Register registers the workflow with the Strait API.
func (w *WorkflowDefinition[TPayload]) Register(ctx context.Context, client WorkflowDSLClient, projectID string) (map[string]any, error) {
	body, err := w.ToRegistrationBody(projectID)
	if err != nil {
		return nil, err
	}

	result, err := client.CreateWorkflow(ctx, body)
	if err != nil {
		return nil, err
	}

	if id, ok := result["id"].(string); ok && id != "" {
		w.lastRegisteredWorkflowID = id
	}

	return result, nil
}

// Trigger triggers a run of this workflow.
func (w *WorkflowDefinition[TPayload]) Trigger(ctx context.Context, client WorkflowDSLClient, input TriggerWorkflowInput[TPayload]) (map[string]any, error) {
	wfID := input.WorkflowID
	if wfID == "" {
		wfID = w.lastRegisteredWorkflowID
	}
	if wfID == "" {
		return nil, fmt.Errorf("defineWorkflow(%s) trigger requires workflowID or prior successful register()", w.Slug)
	}

	payloadBytes, _ := json.Marshal(input.Payload)
	var payload any
	_ = json.Unmarshal(payloadBytes, &payload)

	body := map[string]any{"payload": payload}
	if input.IdempotencyKey != "" {
		body["idempotency_key"] = input.IdempotencyKey
	}
	if input.Priority != nil {
		body["priority"] = *input.Priority
	}
	if input.DryRun != nil {
		body["dry_run"] = *input.DryRun
	}
	if input.Metadata != nil {
		body["metadata"] = input.Metadata
	}
	if input.StepOverrides != nil {
		body["step_overrides"] = input.StepOverrides
	}

	return client.TriggerWorkflow(ctx, wfID, body)
}
