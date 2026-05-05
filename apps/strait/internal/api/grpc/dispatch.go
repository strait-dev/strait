package grpc

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// WorkerDispatcher picks a connected worker, sends a TaskAssignment over the
// gRPC stream, and awaits the TaskResult via a per-run result channel.
// It is called from the executor dispatch path when job.ExecutionMode == "worker".
//
// On no available worker for the run's queue, WorkerDispatch returns
// ErrNoWorkerAvailable so the caller can leave the run queued for the next tick.
type WorkerDispatcher struct {
	registry       *ConnectionRegistry
	queries        *store.Queries
	jwtSigningKey  string
	resultChannels *ResultChannelRegistry
}

// ResultChannelRegistry manages per-run result channels shared between the
// stream recv loop (which writes) and WorkerDispatch (which reads).
//
// Each registered channel is bound to the project ID of the run that owns it.
// Send rejects deliveries from any other project, preventing a worker
// authenticated to project A from completing a run owned by project B with
// forged status / output_json (cross-tenant integrity attack).
type ResultChannelRegistry struct {
	mu       sync.Mutex
	channels map[string]resultChannelEntry
}

type resultChannelEntry struct {
	ch        chan *workerv1.TaskResult
	projectID string
	workerID  string
}

// NewResultChannelRegistry creates an empty registry.
func NewResultChannelRegistry() *ResultChannelRegistry {
	return &ResultChannelRegistry{
		channels: make(map[string]resultChannelEntry),
	}
}

// Register creates a buffered channel for the given run ID, scoped to the
// run's project ID, and returns it. The dispatcher must pass the authoritative
// project ID from the run row (not user input).
func (r *ResultChannelRegistry) Register(runID, projectID, workerID string) chan *workerv1.TaskResult {
	ch := make(chan *workerv1.TaskResult, 1)
	r.mu.Lock()
	r.channels[runID] = resultChannelEntry{ch: ch, projectID: projectID, workerID: workerID}
	r.mu.Unlock()
	return ch
}

// Deregister removes the channel for the given run ID. Must be called by
// WorkerDispatch when the dispatch completes (deferred).
func (r *ResultChannelRegistry) Deregister(runID string) {
	r.mu.Lock()
	delete(r.channels, runID)
	r.mu.Unlock()
}

// Send delivers a TaskResult to the channel for the given run ID, ONLY if the
// caller's project ID matches the project ID the channel was registered with.
// Returns true on successful delivery; false on missing channel, project
// mismatch, or already-buffered duplicate.
func (r *ResultChannelRegistry) Send(runID, projectID, workerID string, result *workerv1.TaskResult) bool {
	r.mu.Lock()
	entry, ok := r.channels[runID]
	r.mu.Unlock()
	if !ok || entry.projectID != projectID || entry.workerID != workerID {
		return false
	}
	select {
	case entry.ch <- result:
		return true
	default:
		// Channel already has a result; discard duplicate.
		return false
	}
}

// ErrNoWorkerAvailable is returned when no connected worker services the run's queue.
var ErrNoWorkerAvailable = fmt.Errorf("no worker available for queue")

// NewWorkerDispatcher creates a WorkerDispatcher. The resultChannels registry
// must be shared with the workerService so TaskResult messages received on the
// stream are routed to the waiting WorkerDispatch call.
func NewWorkerDispatcher(
	registry *ConnectionRegistry,
	queries *store.Queries,
	jwtSigningKey string,
	resultChannels *ResultChannelRegistry,
) *WorkerDispatcher {
	return &WorkerDispatcher{
		registry:       registry,
		queries:        queries,
		jwtSigningKey:  jwtSigningKey,
		resultChannels: resultChannels,
	}
}

// WorkerDispatch assigns a run to the least-loaded active worker for the
// run's queue and project. It:
//  1. Picks a worker via PickWorkerForQueue (most slots available).
//  2. Inserts a worker_tasks row.
//  3. Sends TaskAssignment over the worker's send channel.
//  4. Waits for TaskResult via the per-run buffered result channel.
//  5. On ctx.Done(), sends CancelTask and returns ctx.Err().
//
// Callers are responsible for marking the run terminal and recording cost
// after WorkerDispatch returns.
//
// WorkerDispatch satisfies worker.WorkerRunDispatcher: the return value is
// boxed as interface{} and can be inspected via TaskResultStatus /
// TaskResultError helpers in this package.
func (d *WorkerDispatcher) WorkerDispatch(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
) (any, error) {
	// Atomic pick + slot reservation under the registry write lock. Without
	// this, N concurrent dispatchers can all see SlotsAvailable=1 on the
	// same worker, all decrement, and oversubscribe a single-slot worker.
	workerID, sendCh, ok := d.registry.ReserveWorkerForQueue(run.ProjectID, job.Queue, job.EnvironmentID)
	if !ok {
		return nil, ErrNoWorkerAvailable
	}
	if sendCh == nil {
		// Defensive: a worker entry without a send channel should never be
		// pickable, but if one slipped through, release the reservation.
		d.registry.IncrementSlots(workerID)
		return nil, ErrNoWorkerAvailable
	}

	// Insert worker_tasks record so the reaper can pick it up if the
	// worker disconnects without reporting a result.
	task := &domain.WorkerTask{
		ID:        uuid.Must(uuid.NewV7()).String(),
		WorkerID:  workerID,
		RunID:     run.ID,
		ProjectID: run.ProjectID,
		Status:    domain.WorkerTaskStatusAssigned,
	}
	if err := d.queries.CreateWorkerTask(ctx, task); err != nil {
		d.registry.IncrementSlots(workerID)
		return nil, fmt.Errorf("worker dispatch: record task: %w", err)
	}

	// Register the result channel before sending the assignment so we can
	// never miss a TaskResult that arrives before we start waiting. The
	// channel is bound to run.ProjectID so any TaskResult arriving from a
	// worker authenticated to a different project is dropped on the floor.
	resultCh := d.resultChannels.Register(run.ID, run.ProjectID, workerID)
	defer d.resultChannels.Deregister(run.ID)

	// Build and send the TaskAssignment.
	assignment := d.buildAssignment(run, job)
	msg := &workerv1.ServerMessage{
		Payload: &workerv1.ServerMessage_TaskAssignment{
			TaskAssignment: assignment,
		},
	}

	select {
	case sendCh <- msg:
	case <-ctx.Done():
		d.registry.IncrementSlots(workerID)
		return nil, ctx.Err()
	}

	// Wait for the TaskResult or context cancellation.
	select {
	case result, open := <-resultCh:
		d.registry.IncrementSlots(workerID)
		if !open || result == nil {
			return nil, fmt.Errorf("worker dispatch: result channel closed for run %s", run.ID)
		}
		// Update the worker_tasks row.
		taskStatus := domain.WorkerTaskStatusFailed
		if result.Status == "success" {
			taskStatus = domain.WorkerTaskStatusCompleted
		}
		if err := d.queries.UpdateWorkerTaskStatus(ctx, task.ID, taskStatus); err != nil {
			slog.Warn("worker dispatch: update task status",
				"task_id", task.ID,
				"run_id", run.ID,
				"error", err,
			)
		}
		// Return as interface{} so callers in worker package don't need to import workerv1.
		return result, nil

	case <-ctx.Done():
		// Best-effort cancellation: notify the worker.
		d.sendCancel(sendCh, run.ID)
		d.registry.IncrementSlots(workerID)
		return nil, ctx.Err()
	}
}

// sendCancel sends a CancelTask message to the worker. Non-blocking; errors
// are silently dropped because the worker may have already disconnected.
func (d *WorkerDispatcher) sendCancel(sendCh chan<- *workerv1.ServerMessage, runID string) {
	if sendCh == nil {
		return
	}
	cancelMsg := &workerv1.ServerMessage{
		Payload: &workerv1.ServerMessage_CancelTask{
			CancelTask: &workerv1.CancelTask{
				RunId:  runID,
				Reason: "context cancelled",
			},
		},
	}
	select {
	case sendCh <- cancelMsg:
	default:
	}
}

// buildAssignment constructs a TaskAssignment for the given run and job,
// including JWT run-token and HMAC signature (matching the HTTP dispatch path).
func (d *WorkerDispatcher) buildAssignment(run *domain.JobRun, job *domain.Job) *workerv1.TaskAssignment {
	a := &workerv1.TaskAssignment{
		RunId:       run.ID,
		JobSlug:     job.Slug,
		Queue:       job.Queue,
		PayloadJson: run.Payload,
		TimeoutSecs: int32(job.TimeoutSecs), //nolint:gosec // TimeoutSecs is validated upstream to be non-negative and within range
	}

	// JWT run-token so the worker SDK can authenticate callbacks.
	if d.jwtSigningKey != "" {
		expiresAt := time.Now().Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
		if run.ExpiresAt != nil {
			expiresAt = *run.ExpiresAt
		}
		claims := jwt.RegisteredClaims{
			Issuer:    "strait:run-token",
			Subject:   run.ID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		if signed, err := tok.SignedString([]byte(d.jwtSigningKey)); err == nil {
			a.RunTokenJwt = signed
		}
	}

	// HMAC-SHA256 body+timestamp signing (same algorithm as worker.SignHTTPDispatch
	// in internal/worker/sign.go — reproduced here to avoid circular import).
	if job.EndpointSigningSecret != "" {
		ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
		a.HmacTimestamp = ts
		a.HmacSignature = dispatchHMAC(job.EndpointSigningSecret, ts, run.Payload)
	}

	return a
}

// dispatchHMAC returns `v1=<hex-sha256>` for the given secret, timestamp, and
// body. Matches worker.SignHTTPDispatch exactly.
func dispatchHMAC(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

// TaskResultStatus returns the status string from an opaque TaskResult
// returned by WorkerDispatch. Returns "" on nil or wrong type.
func TaskResultStatus(opaque any) string {
	if opaque == nil {
		return ""
	}
	r, ok := opaque.(*workerv1.TaskResult)
	if !ok {
		return ""
	}
	return r.Status
}

// TaskResultError returns the error message from an opaque TaskResult.
func TaskResultError(opaque any) string {
	if opaque == nil {
		return ""
	}
	r, ok := opaque.(*workerv1.TaskResult)
	if !ok {
		return ""
	}
	return r.ErrorMessage
}

// TaskResultOutput returns output_json from an opaque TaskResult.
// A copy is returned so callers can safely retain it after the proto object is reused.
func TaskResultOutput(opaque any) json.RawMessage {
	if opaque == nil {
		return nil
	}
	r, ok := opaque.(*workerv1.TaskResult)
	if !ok || len(r.OutputJson) == 0 {
		return nil
	}
	out := make([]byte, len(r.OutputJson))
	copy(out, r.OutputJson)
	return json.RawMessage(out)
}

// ResultStatus implements worker.WorkerRunDispatcher by delegating to
// TaskResultStatus. Defined as a method so the worker package can extract
// the status from the opaque result without importing grpc proto types.
func (d *WorkerDispatcher) ResultStatus(opaque any) string {
	return TaskResultStatus(opaque)
}

// ResultError implements worker.WorkerRunDispatcher by delegating to
// TaskResultError.
func (d *WorkerDispatcher) ResultError(opaque any) string {
	return TaskResultError(opaque)
}

// ResultOutput implements worker.WorkerRunDispatcher by delegating to
// TaskResultOutput.
func (d *WorkerDispatcher) ResultOutput(opaque any) json.RawMessage {
	return TaskResultOutput(opaque)
}
