package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"orchestrator/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrJobNotFound             = errors.New("job not found")
	ErrRunNotFound             = errors.New("run not found")
	ErrRunConflict             = errors.New("run status update conflict")
	ErrWorkflowNotFound        = errors.New("workflow not found")
	ErrWorkflowStepNotFound    = errors.New("workflow step not found")
	ErrWorkflowRunNotFound     = errors.New("workflow run not found")
	ErrWorkflowStepRunNotFound = errors.New("workflow step run not found")
)

type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type JobStore interface {
	CreateJob(ctx context.Context, job *domain.Job) error
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error)
	ListJobs(ctx context.Context, projectID string) ([]domain.Job, error)
	UpdateJob(ctx context.Context, job *domain.Job) error
	DeleteJob(ctx context.Context, id string) error
	ListCronJobs(ctx context.Context) ([]domain.Job, error)
}

type RunStore interface {
	CreateRun(ctx context.Context, run *domain.JobRun) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error)
	ListRunsByJob(ctx context.Context, jobID string, limit, offset int) ([]domain.JobRun, error)
	ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, limit int, cursor *time.Time) ([]domain.JobRun, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	UpdateHeartbeat(ctx context.Context, id string) error
	ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
	ListDueRuns(ctx context.Context) ([]domain.JobRun, error)
	ListExpiredRuns(ctx context.Context) ([]domain.JobRun, error)
	ListChildRuns(ctx context.Context, parentRunID string) ([]domain.JobRun, error)
	ListStaleDequeued(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
}

type EventStore interface {
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	ListEvents(ctx context.Context, runID string) ([]domain.RunEvent, error)
	ListEventsByRunFiltered(ctx context.Context, runID string, level, eventType string) ([]domain.RunEvent, error)
}

type WebhookDeliveryStore interface {
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	UpdateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	ListWebhookDeliveries(ctx context.Context, projectID, status string, limit int) ([]domain.WebhookDelivery, error)
	GetWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error)
	ListPendingWebhookRetries(ctx context.Context) ([]domain.WebhookDelivery, error)
}

type APIKeyStore interface {
	CreateAPIKey(ctx context.Context, key *domain.APIKey) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error)
	ListAPIKeysByProject(ctx context.Context, projectID string) ([]domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, id string) error
	TouchAPIKeyLastUsed(ctx context.Context, id string) error
}

type JobVersionStore interface {
	CreateJobVersion(ctx context.Context, v *domain.JobVersion) error
	ListJobVersionsByJob(ctx context.Context, jobID string) ([]domain.JobVersion, error)
	GetJobVersion(ctx context.Context, jobID string, version int) (*domain.JobVersion, error)
}

type WorkflowStore interface {
	CreateWorkflow(ctx context.Context, w *domain.Workflow) error
	GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error)
	GetWorkflowBySlug(ctx context.Context, projectID, slug string) (*domain.Workflow, error)
	ListWorkflows(ctx context.Context, projectID string) ([]domain.Workflow, error)
	ListCronWorkflows(ctx context.Context) ([]domain.Workflow, error)
	UpdateWorkflow(ctx context.Context, w *domain.Workflow) error
	CreateWorkflowVersionSnapshot(ctx context.Context, workflowID string, version int) error
	ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	CountRunningWorkflowRuns(ctx context.Context, workflowID string) (int, error)
	DeleteWorkflow(ctx context.Context, id string) error
}

type WorkflowStepStore interface {
	CreateWorkflowStep(ctx context.Context, step *domain.WorkflowStep) error
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]domain.WorkflowStep, error)
	GetWorkflowStep(ctx context.Context, id string) (*domain.WorkflowStep, error)
	DeleteStepsByWorkflow(ctx context.Context, workflowID string) error
}

// StepDepResult is returned by IncrementStepDeps for each step whose deps_completed was incremented.
type StepDepResult struct {
	StepRunID     string
	StepRef       string
	DepsCompleted int
	DepsRequired  int
	JobID         string
	Condition     json.RawMessage
	Payload       json.RawMessage
	WorkflowRunID string
}

type WorkflowRunStore interface {
	CreateWorkflowRun(ctx context.Context, run *domain.WorkflowRun) error
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	ListWorkflowRuns(ctx context.Context, workflowID string, limit, offset int) ([]domain.WorkflowRun, error)
	ListWorkflowRunsByProject(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int) ([]domain.WorkflowRun, error)
	CreateWorkflowRunLabels(ctx context.Context, workflowRunID string, labels map[string]string) error
	ListWorkflowRunLabels(ctx context.Context, workflowRunID string) (map[string]string, error)
	DeleteWorkflowRunsFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error)
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
	ListTimedOutWorkflowRuns(ctx context.Context) ([]domain.WorkflowRun, error)
}

type WorkflowStepRunStore interface {
	CreateWorkflowStepRun(ctx context.Context, sr *domain.WorkflowStepRun) error
	GetWorkflowStepRun(ctx context.Context, id string) (*domain.WorkflowStepRun, error)
	GetStepRunByWorkflowRunAndRef(ctx context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error)
	GetStepRunByJobRunID(ctx context.Context, jobRunID string) (*domain.WorkflowStepRun, error)
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error)
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	IncrementStepDeps(ctx context.Context, workflowRunID string, completedStepRef string) ([]StepDepResult, error)
	GetStepOutputs(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error)
	CreateWorkflowStepApproval(ctx context.Context, approval *domain.WorkflowStepApproval) error
	GetWorkflowStepApprovalByStepRunID(ctx context.Context, stepRunID string) (*domain.WorkflowStepApproval, error)
	UpdateWorkflowStepApproval(ctx context.Context, id string, status string, approvedBy string, approvedAt *time.Time, errMsg string) error
	ListExpiredWorkflowStepApprovals(ctx context.Context) ([]domain.WorkflowStepApproval, error)
	IncrementStepRunAttempt(ctx context.Context, id string, newAttempt int) error
}

type Store interface {
	JobStore
	RunStore
	EventStore
	WebhookDeliveryStore
	APIKeyStore
	JobVersionStore
	WorkflowStore
	WorkflowStepStore
	WorkflowRunStore
	WorkflowStepRunStore
	QueueStats(ctx context.Context) (*QueueStats, error)
}

type Queries struct {
	db DBTX
}

func New(db DBTX) *Queries {
	return &Queries{db: db}
}
