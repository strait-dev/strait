package worker

import (
	"context"
	"encoding/json"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// ExecutorStore is the subset of store operations needed by Executor.
type ExecutorStore interface {
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	GetJobAtVersion(ctx context.Context, jobID string, version int) (*domain.Job, error)
	GetWorkflowStepRun(ctx context.Context, id string) (*domain.WorkflowStepRun, error)
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	ListJobSecretsByJob(ctx context.Context, jobID, environment string) ([]domain.JobSecret, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	SnoozeRunWithLock(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	UpdateHeartbeat(ctx context.Context, id string) error
	ScheduleRetry(ctx context.Context, runID string, at time.Time, attempt int) error
	ClearRetry(ctx context.Context, runID string) error
	CanDispatchEndpoint(ctx context.Context, endpointURL string, now time.Time) (bool, *time.Time, error)
	RecordEndpointCircuitFailure(
		ctx context.Context,
		endpointURL string,
		now time.Time,
		threshold int,
		openDuration time.Duration,
	) error
	RecordEndpointCircuitSuccess(ctx context.Context, endpointURL string) error
	GetJobHealthStats(ctx context.Context, jobID string, since time.Time) (*store.JobHealthStats, error)
	GetResolvedEnvironmentVariables(ctx context.Context, projectID, id string) (map[string]string, error)
	GetLatestCheckpoint(ctx context.Context, runID string) (*domain.RunCheckpoint, error)
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	GetEndpointHealthScore(ctx context.Context, endpointURL string) (*domain.EndpointHealthScore, error)
	UpsertEndpointHealthScore(ctx context.Context, score *domain.EndpointHealthScore) error
	AtomicRecordHealthResult(
		ctx context.Context,
		endpointURL string,
		successVal, timeoutVal, latencyVal, alpha float64,
		weightSuccess, weightTimeout, weightLatency float64,
		lastLatencyMs float64,
	) (*domain.EndpointHealthScore, error)
	CountExecutingRunsByOrg(ctx context.Context, orgID string) (int, error)
}

type existingRunEnqueuer interface {
	EnqueueExisting(ctx context.Context, run *domain.JobRun) error
}

type executionPolicy struct {
	maxAttempts      int
	timeoutSecs      int
	retryBackoff     domain.RetryBackoffPolicy
	retryInitialSecs int
	retryMaxSecs     int
}

// WorkflowCallback is called after a job run reaches a terminal state.
// Nil-safe: if nil, no callback is invoked.
type WorkflowCallback interface {
	OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
}

// WorkerRunDispatcher is implemented by grpc.WorkerDispatcher to avoid a
// circular import between internal/worker and internal/api/grpc. Injected
// via ExecutorConfig.WorkerDispatcher.
//
// The return value is an opaque result container; callers cast it to extract
// the status and error message fields they need. Defined as interface{} to
// keep the worker package free of grpc proto imports; the actual type is
// *workerv1.TaskResult.
type WorkerRunDispatcher interface {
	WorkerDispatch(ctx context.Context, run *domain.JobRun, job *domain.Job) (any, error)
	// ResultStatus extracts the status string ("success", "failed", or "")
	// from an opaque TaskResult. Returns "" for nil or wrong type.
	ResultStatus(opaque any) string
	// ResultError extracts the error message from a failed TaskResult.
	// Returns "" for nil, wrong type, or empty error_message.
	ResultError(opaque any) string
	// ResultOutput extracts output_json from a successful TaskResult.
	// Returns nil for nil, wrong type, or empty output_json.
	ResultOutput(opaque any) json.RawMessage
}

type workerTaskCompletionDispatcher interface {
	CompleteWorkerTask(ctx context.Context, opaque any, status domain.WorkerTaskStatus) error
}

type SecretDecryptor interface {
	Decrypt([]byte) ([]byte, error)
}

type ConcurrencyLimitProvider interface {
	CurrentLimit() int
}
