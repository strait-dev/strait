package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	GetProjectQuota(ctx context.Context, projectID string) (*ProjectQuota, error)
	CountProjectQueuedRuns(ctx context.Context, projectID string) (int, error)
	CountProjectActiveRuns(ctx context.Context, projectID string) (int, error)
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
	DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error)
	FindRecentRunByPayload(ctx context.Context, jobID string, payload json.RawMessage, since time.Time) (*domain.JobRun, error)
	CountRunsForJobSince(ctx context.Context, jobID string, since time.Time) (int, error)
	CreateRunCheckpoint(ctx context.Context, checkpoint *domain.RunCheckpoint) error
	ListRunCheckpoints(ctx context.Context, runID string, limit int) ([]domain.RunCheckpoint, error)
	CreateRunUsage(ctx context.Context, usage *domain.RunUsage) error
	ListRunUsage(ctx context.Context, runID string, limit int) ([]domain.RunUsage, error)
	CreateRunToolCall(ctx context.Context, call *domain.RunToolCall) error
	ListRunToolCalls(ctx context.Context, runID string, limit int) ([]domain.RunToolCall, error)
	UpsertRunOutput(ctx context.Context, output *domain.RunOutput) error
	ListRunOutputs(ctx context.Context, runID string) ([]domain.RunOutput, error)
	AreAllDescendantsTerminal(ctx context.Context, parentRunID string) (bool, error)
	GetEndpointCircuitState(ctx context.Context, endpointURL string) (*domain.EndpointCircuitState, error)
	CanDispatchEndpoint(ctx context.Context, endpointURL string, now time.Time) (bool, *time.Time, error)
	RecordEndpointCircuitFailure(ctx context.Context, endpointURL string, now time.Time, threshold int, openDuration time.Duration) error
	RecordEndpointCircuitSuccess(ctx context.Context, endpointURL string) error
}

type ProjectQuota struct {
	ProjectID        string
	MaxQueuedRuns    int
	MaxExecutingRuns int
	MaxJobs          int
	Timezone         string
}

type EventStore interface {
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	ListEvents(ctx context.Context, runID string) ([]domain.RunEvent, error)
	ListEventsByRunFiltered(ctx context.Context, runID string, level, eventType string) ([]domain.RunEvent, error)
}

type WebhookDeliveryStore interface {
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	UpdateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	ListWebhookDeliveries(ctx context.Context, status string, limit int) ([]domain.WebhookDelivery, error)
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
	UpdateWorkflow(ctx context.Context, w *domain.Workflow) error
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
	UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error
}

type WorkflowStepRunStore interface {
	CreateWorkflowStepRun(ctx context.Context, sr *domain.WorkflowStepRun) error
	GetWorkflowStepRun(ctx context.Context, id string) (*domain.WorkflowStepRun, error)
	GetStepRunByJobRunID(ctx context.Context, jobRunID string) (*domain.WorkflowStepRun, error)
	ListStepRunsByWorkflowRun(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error)
	UpdateStepRunStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error
	IncrementStepDeps(ctx context.Context, workflowRunID string, completedStepRef string) ([]StepDepResult, error)
	GetStepOutputs(ctx context.Context, workflowRunID string, stepRefs []string) (map[string]json.RawMessage, error)
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

type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

func WithTx(ctx context.Context, db TxBeginner, fn func(q *Queries) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		_ = tx.Rollback(ctx)
	}()

	if err := fn(New(tx)); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	committed = true

	return nil
}
