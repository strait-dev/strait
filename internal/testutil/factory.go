//go:build integration

package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/queue"
	"orchestrator/internal/store"

	"github.com/google/uuid"
)

var seq int64

func nextSeq() int64 { return atomic.AddInt64(&seq, 1) }

type JobOpts struct {
	ID            *string
	ProjectID     *string
	Name          *string
	Slug          *string
	Description   *string
	Cron          *string
	PayloadSchema []byte
	EndpointURL   *string
	MaxAttempts   *int
	TimeoutSecs   *int
	Enabled       *bool
	WebhookURL    *string
	WebhookSecret *string
}

func BuildJob(opts *JobOpts) *domain.Job {
	n := nextSeq()
	job := &domain.Job{
		ID:            uuid.Must(uuid.NewV7()).String(),
		ProjectID:     fmt.Sprintf("project-%d", n),
		Name:          fmt.Sprintf("job-%d", n),
		Slug:          fmt.Sprintf("slug-%d", n),
		PayloadSchema: json.RawMessage(`{"type":"object"}`),
		EndpointURL:   "https://example.com/webhook",
		MaxAttempts:   3,
		TimeoutSecs:   300,
		Enabled:       true,
	}

	if opts == nil {
		return job
	}

	if opts.ID != nil {
		job.ID = *opts.ID
	}
	if opts.ProjectID != nil {
		job.ProjectID = *opts.ProjectID
	}
	if opts.Name != nil {
		job.Name = *opts.Name
	}
	if opts.Slug != nil {
		job.Slug = *opts.Slug
	}
	if opts.Description != nil {
		job.Description = *opts.Description
	}
	if opts.Cron != nil {
		job.Cron = *opts.Cron
	}
	if opts.PayloadSchema != nil {
		job.PayloadSchema = append([]byte(nil), opts.PayloadSchema...)
	}
	if opts.EndpointURL != nil {
		job.EndpointURL = *opts.EndpointURL
	}
	if opts.MaxAttempts != nil {
		job.MaxAttempts = *opts.MaxAttempts
	}
	if opts.TimeoutSecs != nil {
		job.TimeoutSecs = *opts.TimeoutSecs
	}
	if opts.Enabled != nil {
		job.Enabled = *opts.Enabled
	}
	if opts.WebhookURL != nil {
		job.WebhookURL = *opts.WebhookURL
	}
	if opts.WebhookSecret != nil {
		job.WebhookSecret = *opts.WebhookSecret
	}

	return job
}

func MustCreateJob(t testing.TB, ctx context.Context, s store.Store, opts *JobOpts) *domain.Job {
	t.Helper()

	job := BuildJob(opts)
	if err := s.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	return job
}

type RunOpts struct {
	ID             *string
	Status         *domain.RunStatus
	Attempt        *int
	Payload        []byte
	TriggeredBy    *string
	ScheduledAt    *time.Time
	Priority       *int
	IdempotencyKey *string
	ParentRunID    *string
}

func BuildRun(job *domain.Job, opts *RunOpts) *domain.JobRun {
	run := &domain.JobRun{
		ID:          uuid.Must(uuid.NewV7()).String(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Status:      "",
		Attempt:     1,
		Payload:     json.RawMessage(`{"test":true}`),
		TriggeredBy: "manual",
		Priority:    0,
	}

	if opts == nil {
		return run
	}

	if opts.ID != nil {
		run.ID = *opts.ID
	}
	if opts.Status != nil {
		run.Status = *opts.Status
	}
	if opts.Attempt != nil {
		run.Attempt = *opts.Attempt
	}
	if opts.Payload != nil {
		run.Payload = append([]byte(nil), opts.Payload...)
	}
	if opts.TriggeredBy != nil {
		run.TriggeredBy = *opts.TriggeredBy
	}
	if opts.ScheduledAt != nil {
		t := *opts.ScheduledAt
		run.ScheduledAt = &t
	}
	if opts.Priority != nil {
		run.Priority = *opts.Priority
	}
	if opts.IdempotencyKey != nil {
		run.IdempotencyKey = *opts.IdempotencyKey
	}
	if opts.ParentRunID != nil {
		run.ParentRunID = *opts.ParentRunID
	}

	return run
}

func MustCreateRun(t testing.TB, ctx context.Context, s store.Store, job *domain.Job, opts *RunOpts) *domain.JobRun {
	t.Helper()

	run := BuildRun(job, opts)
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	return run
}

func MustEnqueueRun(t testing.TB, ctx context.Context, q queue.Queue, job *domain.Job, opts *RunOpts) *domain.JobRun {
	t.Helper()

	run := BuildRun(job, opts)
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	return run
}

type WorkflowOpts struct {
	ID          *string
	ProjectID   *string
	Name        *string
	Slug        *string
	Description *string
	Enabled     *bool
	Version     *int
}

func BuildWorkflow(opts *WorkflowOpts) *domain.Workflow {
	n := nextSeq()
	wf := &domain.Workflow{
		ID:        uuid.Must(uuid.NewV7()).String(),
		ProjectID: fmt.Sprintf("project-%d", n),
		Name:      fmt.Sprintf("workflow-%d", n),
		Slug:      fmt.Sprintf("wf-slug-%d", n),
		Enabled:   true,
		Version:   1,
	}

	if opts == nil {
		return wf
	}

	if opts.ID != nil {
		wf.ID = *opts.ID
	}
	if opts.ProjectID != nil {
		wf.ProjectID = *opts.ProjectID
	}
	if opts.Name != nil {
		wf.Name = *opts.Name
	}
	if opts.Slug != nil {
		wf.Slug = *opts.Slug
	}
	if opts.Description != nil {
		wf.Description = *opts.Description
	}
	if opts.Enabled != nil {
		wf.Enabled = *opts.Enabled
	}
	if opts.Version != nil {
		wf.Version = *opts.Version
	}

	return wf
}

func MustCreateWorkflow(t testing.TB, ctx context.Context, s store.Store, opts *WorkflowOpts) *domain.Workflow {
	t.Helper()

	wf := BuildWorkflow(opts)
	if err := s.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	return wf
}

type WorkflowStepOpts struct {
	ID         *string
	WorkflowID *string
	JobID      *string
	StepRef    *string
	DependsOn  []string
	Condition  json.RawMessage
	OnFailure  *domain.FailurePolicy
	Payload    json.RawMessage
}

func BuildWorkflowStep(workflowID string, opts *WorkflowStepOpts) *domain.WorkflowStep {
	n := nextSeq()
	step := &domain.WorkflowStep{
		ID:         uuid.Must(uuid.NewV7()).String(),
		WorkflowID: workflowID,
		JobID:      uuid.Must(uuid.NewV7()).String(),
		StepRef:    fmt.Sprintf("step-%d", n),
	}

	if opts == nil {
		return step
	}

	if opts.ID != nil {
		step.ID = *opts.ID
	}
	if opts.WorkflowID != nil {
		step.WorkflowID = *opts.WorkflowID
	}
	if opts.JobID != nil {
		step.JobID = *opts.JobID
	}
	if opts.StepRef != nil {
		step.StepRef = *opts.StepRef
	}
	if opts.DependsOn != nil {
		step.DependsOn = append([]string(nil), opts.DependsOn...)
	}
	if opts.Condition != nil {
		step.Condition = append(json.RawMessage(nil), opts.Condition...)
	}
	if opts.OnFailure != nil {
		step.OnFailure = *opts.OnFailure
	}
	if opts.Payload != nil {
		step.Payload = append(json.RawMessage(nil), opts.Payload...)
	}

	return step
}

func MustCreateWorkflowStep(t testing.TB, ctx context.Context, s store.Store, workflowID string, opts *WorkflowStepOpts) *domain.WorkflowStep {
	t.Helper()

	step := BuildWorkflowStep(workflowID, opts)
	if err := s.CreateWorkflowStep(ctx, step); err != nil {
		t.Fatalf("CreateWorkflowStep() error = %v", err)
	}

	return step
}

type WorkflowRunOpts struct {
	ID          *string
	WorkflowID  *string
	ProjectID   *string
	Status      *domain.WorkflowRunStatus
	TriggeredBy *string
	Payload     json.RawMessage
}

func BuildWorkflowRun(workflowID string, opts *WorkflowRunOpts) *domain.WorkflowRun {
	n := nextSeq()
	wfRun := &domain.WorkflowRun{
		ID:          uuid.Must(uuid.NewV7()).String(),
		WorkflowID:  workflowID,
		ProjectID:   fmt.Sprintf("project-%d", n),
		Status:      domain.WfStatusPending,
		TriggeredBy: domain.TriggerManual,
	}

	if opts == nil {
		return wfRun
	}

	if opts.ID != nil {
		wfRun.ID = *opts.ID
	}
	if opts.WorkflowID != nil {
		wfRun.WorkflowID = *opts.WorkflowID
	}
	if opts.ProjectID != nil {
		wfRun.ProjectID = *opts.ProjectID
	}
	if opts.Status != nil {
		wfRun.Status = *opts.Status
	}
	if opts.TriggeredBy != nil {
		wfRun.TriggeredBy = *opts.TriggeredBy
	}
	if opts.Payload != nil {
		wfRun.Payload = append(json.RawMessage(nil), opts.Payload...)
	}

	return wfRun
}

func MustCreateWorkflowRun(t testing.TB, ctx context.Context, s store.Store, workflowID string, opts *WorkflowRunOpts) *domain.WorkflowRun {
	t.Helper()

	wfRun := BuildWorkflowRun(workflowID, opts)
	if err := s.CreateWorkflowRun(ctx, wfRun); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	return wfRun
}

type WorkflowStepRunOpts struct {
	ID             *string
	WorkflowRunID  *string
	WorkflowStepID *string
	StepRef        *string
	JobRunID       *string
	Status         *domain.StepRunStatus
	DepsCompleted  *int
	DepsRequired   *int
	Output         json.RawMessage
	Error          *string
}

func BuildWorkflowStepRun(workflowRunID, workflowStepID string, opts *WorkflowStepRunOpts) *domain.WorkflowStepRun {
	n := nextSeq()
	sr := &domain.WorkflowStepRun{
		ID:             uuid.Must(uuid.NewV7()).String(),
		WorkflowRunID:  workflowRunID,
		WorkflowStepID: workflowStepID,
		StepRef:        fmt.Sprintf("step-%d", n),
		Status:         domain.StepPending,
	}

	if opts == nil {
		return sr
	}

	if opts.ID != nil {
		sr.ID = *opts.ID
	}
	if opts.WorkflowRunID != nil {
		sr.WorkflowRunID = *opts.WorkflowRunID
	}
	if opts.WorkflowStepID != nil {
		sr.WorkflowStepID = *opts.WorkflowStepID
	}
	if opts.StepRef != nil {
		sr.StepRef = *opts.StepRef
	}
	if opts.JobRunID != nil {
		sr.JobRunID = *opts.JobRunID
	}
	if opts.Status != nil {
		sr.Status = *opts.Status
	}
	if opts.DepsCompleted != nil {
		sr.DepsCompleted = *opts.DepsCompleted
	}
	if opts.DepsRequired != nil {
		sr.DepsRequired = *opts.DepsRequired
	}
	if opts.Output != nil {
		sr.Output = append(json.RawMessage(nil), opts.Output...)
	}
	if opts.Error != nil {
		sr.Error = *opts.Error
	}

	return sr
}

func MustCreateWorkflowStepRun(t testing.TB, ctx context.Context, s store.Store, workflowRunID, workflowStepID string, opts *WorkflowStepRunOpts) *domain.WorkflowStepRun {
	t.Helper()

	sr := BuildWorkflowStepRun(workflowRunID, workflowStepID, opts)
	if err := s.CreateWorkflowStepRun(ctx, sr); err != nil {
		t.Fatalf("CreateWorkflowStepRun() error = %v", err)
	}

	return sr
}

type EventOpts struct {
	ID      *string
	Type    *string
	Level   *string
	Message *string
	Data    []byte
}

func MustCreateEvent(t testing.TB, ctx context.Context, s store.Store, runID string, opts *EventOpts) *domain.RunEvent {
	t.Helper()

	event := &domain.RunEvent{
		ID:      uuid.Must(uuid.NewV7()).String(),
		RunID:   runID,
		Type:    domain.EventType("log"),
		Level:   "info",
		Message: "test event",
		Data:    json.RawMessage(`{}`),
	}

	if opts != nil {
		if opts.ID != nil {
			event.ID = *opts.ID
		}
		if opts.Type != nil {
			event.Type = domain.EventType(*opts.Type)
		}
		if opts.Level != nil {
			event.Level = *opts.Level
		}
		if opts.Message != nil {
			event.Message = *opts.Message
		}
		if opts.Data != nil {
			event.Data = append([]byte(nil), opts.Data...)
		}
	}

	if err := s.InsertEvent(ctx, event); err != nil {
		t.Fatalf("InsertEvent() error = %v", err)
	}

	return event
}

func Ptr[T any](v T) *T { return &v }
