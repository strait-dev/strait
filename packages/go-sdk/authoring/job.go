package authoring

import (
	"context"
	"encoding/json"
	"fmt"
)

// JobOptions configures a job definition.
type JobOptions[TPayload any] struct {
	Name        string
	Slug        string
	EndpointURL string
	ProjectID   string

	Description         string
	GroupID             string
	Tags                map[string]string
	EnvironmentID       string
	Cron                string
	Timezone            string
	ExecutionWindowCron string
	MaxConcurrency      *int
	RateLimitMax        *int
	RateLimitWindowSecs *int
	MaxAttempts         *int
	RetryStrategy       string
	RetryDelaysSecs     []int
	TimeoutSecs         *int
	RunTTLSecs          *int
	DedupWindowSecs     *int
	WebhookURL          string
	WebhookSecret       string
	FallbackEndpointURL string

	// Run is the handler executed when the job runs. Stored but not invoked by the SDK.
	Run func(payload TPayload, ctx RunContext) (any, error)
	// OnSuccess is called after a successful run. Stored but not invoked by the SDK.
	OnSuccess func(payload TPayload, output any, ctx RunContext) error
	// OnFailure is called when a run fails. Stored but not invoked by the SDK.
	OnFailure func(payload TPayload, err error, ctx RunContext) error
	// OnStart is called when a run starts. Stored but not invoked by the SDK.
	OnStart func(payload TPayload, ctx RunContext) error
}

// JobDefinition is a defined job with registration and triggering methods.
type JobDefinition[TPayload any] struct {
	Kind string
	Slug string
	opts JobOptions[TPayload]

	Run       func(payload TPayload, ctx RunContext) (any, error)
	OnSuccess func(payload TPayload, output any, ctx RunContext) error
	OnFailure func(payload TPayload, err error, ctx RunContext) error
	OnStart   func(payload TPayload, ctx RunContext) error

	lastRegisteredJobID string
}

// TriggerJobInput is the input for triggering a job run.
type TriggerJobInput[TPayload any] struct {
	JobID          string
	Payload        TPayload
	IdempotencyKey string
	Priority       *int
	DryRun         *bool
	Metadata       map[string]string
	ScheduledAt    string
}

// JobDSLClient abstracts the API operations needed by job definitions.
type JobDSLClient interface {
	CreateJob(ctx context.Context, body map[string]any) (map[string]any, error)
	TriggerJob(ctx context.Context, jobID string, body map[string]any) (map[string]any, error)
	BulkTriggerJob(ctx context.Context, jobID string, body map[string]any) (map[string]any, error)
	GetRun(ctx context.Context, runID string) (map[string]any, error)
}

// DefineJob creates a new job definition.
func DefineJob[TPayload any](opts JobOptions[TPayload]) *JobDefinition[TPayload] {
	return &JobDefinition[TPayload]{
		Kind:      "job",
		Slug:      opts.Slug,
		opts:      opts,
		Run:       opts.Run,
		OnSuccess: opts.OnSuccess,
		OnFailure: opts.OnFailure,
		OnStart:   opts.OnStart,
	}
}

// ToRegistrationBody builds the snake_case API registration body.
func (j *JobDefinition[TPayload]) ToRegistrationBody(projectID string) (map[string]any, error) {
	pid := j.opts.ProjectID
	if projectID != "" {
		pid = projectID
	}
	if pid == "" {
		return nil, fmt.Errorf("defineJob(%s) requires projectId", j.Slug)
	}

	body := map[string]any{
		"project_id":   pid,
		"name":         j.opts.Name,
		"slug":         j.opts.Slug,
		"endpoint_url": j.opts.EndpointURL,
	}

	setOptStr(body, "description", j.opts.Description)
	setOptStr(body, "group_id", j.opts.GroupID)
	setOptMap(body, "tags", j.opts.Tags)
	setOptStr(body, "environment_id", j.opts.EnvironmentID)
	setOptStr(body, "cron", j.opts.Cron)
	setOptStr(body, "timezone", j.opts.Timezone)
	setOptStr(body, "execution_window_cron", j.opts.ExecutionWindowCron)
	setOptInt(body, "max_concurrency", j.opts.MaxConcurrency)
	setOptInt(body, "rate_limit_max", j.opts.RateLimitMax)
	setOptInt(body, "rate_limit_window_secs", j.opts.RateLimitWindowSecs)
	setOptInt(body, "max_attempts", j.opts.MaxAttempts)
	setOptStr(body, "retry_strategy", j.opts.RetryStrategy)
	if len(j.opts.RetryDelaysSecs) > 0 {
		body["retry_delays_secs"] = j.opts.RetryDelaysSecs
	}
	setOptInt(body, "timeout_secs", j.opts.TimeoutSecs)
	setOptInt(body, "run_ttl_secs", j.opts.RunTTLSecs)
	setOptInt(body, "dedup_window_secs", j.opts.DedupWindowSecs)
	setOptStr(body, "webhook_url", j.opts.WebhookURL)
	setOptStr(body, "webhook_secret", j.opts.WebhookSecret)
	setOptStr(body, "fallback_endpoint_url", j.opts.FallbackEndpointURL)

	return body, nil
}

// Register registers the job with the Strait API.
func (j *JobDefinition[TPayload]) Register(ctx context.Context, client JobDSLClient, projectID string) (map[string]any, error) {
	body, err := j.ToRegistrationBody(projectID)
	if err != nil {
		return nil, err
	}

	result, err := client.CreateJob(ctx, body)
	if err != nil {
		return nil, err
	}

	if id, ok := result["id"].(string); ok && id != "" {
		j.lastRegisteredJobID = id
	}

	return result, nil
}

// Trigger triggers a run of this job.
func (j *JobDefinition[TPayload]) Trigger(ctx context.Context, client JobDSLClient, input TriggerJobInput[TPayload]) (map[string]any, error) {
	jobID := input.JobID
	if jobID == "" {
		jobID = j.lastRegisteredJobID
	}
	if jobID == "" {
		return nil, fmt.Errorf("defineJob(%s) trigger requires jobID or prior successful register()", j.Slug)
	}

	body := buildTriggerBody(input)
	return client.TriggerJob(ctx, jobID, body)
}

// BatchTrigger triggers multiple runs in a single API call.
func (j *JobDefinition[TPayload]) BatchTrigger(ctx context.Context, client JobDSLClient, jobID string, items []TriggerJobInput[TPayload]) (map[string]any, error) {
	if jobID == "" {
		jobID = j.lastRegisteredJobID
	}
	if jobID == "" {
		return nil, fmt.Errorf("defineJob(%s) batchTrigger requires jobID", j.Slug)
	}

	triggerItems := make([]map[string]any, len(items))
	for i, item := range items {
		triggerItems[i] = buildTriggerBody(item)
	}

	return client.BulkTriggerJob(ctx, jobID, map[string]any{"items": triggerItems})
}

func buildTriggerBody[TPayload any](input TriggerJobInput[TPayload]) map[string]any {
	// Convert payload to map via JSON round-trip
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
	if input.ScheduledAt != "" {
		body["scheduled_at"] = input.ScheduledAt
	}
	return body
}

func setOptStr(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

func setOptInt(m map[string]any, key string, val *int) {
	if val != nil {
		m[key] = *val
	}
}

func setOptMap[V any](m map[string]any, key string, val map[string]V) {
	if len(val) > 0 {
		m[key] = val
	}
}
