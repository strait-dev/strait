package grpc

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strconv"
	"sync"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	straitcrypto "strait/internal/crypto"
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
	registry        *ConnectionRegistry
	queries         *store.Queries
	jwtSigningKey   string
	resultChannels  *ResultChannelRegistry
	secretDecryptor SecretDecryptor
}

type SecretDecryptor interface {
	Decrypt([]byte) ([]byte, error)
}

type WorkerTaskResult struct {
	TaskID string
	Result *workerv1.TaskResult
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
	runLocks [256]sync.Mutex
}

type resultChannelEntry struct {
	ch           chan *workerv1.TaskResult
	projectID    string
	workerID     string
	assignmentID string
	attempt      int
}

var ErrResultChannelAlreadyRegistered = errors.New("result channel already registered")

// NewResultChannelRegistry creates an empty registry.
func NewResultChannelRegistry() *ResultChannelRegistry {
	return &ResultChannelRegistry{
		channels: make(map[string]resultChannelEntry),
	}
}

func (d *WorkerDispatcher) WithSecretDecryptor(dec SecretDecryptor) *WorkerDispatcher {
	d.secretDecryptor = dec
	return d
}

// Register creates a buffered channel for the given run ID, scoped to the
// run's project ID, and returns it. The dispatcher must pass the authoritative
// project ID from the run row (not user input).
func (r *ResultChannelRegistry) Register(runID, projectID, workerID, assignmentID string, attempt int) chan *workerv1.TaskResult {
	ch, _ := r.TryRegister(runID, projectID, workerID, assignmentID, attempt)
	return ch
}

// TryRegister creates a result channel unless the run already has an active
// waiter. Duplicate registration is rejected so concurrent dispatch attempts
// for the same run cannot overwrite the original channel and orphan its
// dispatcher.
func (r *ResultChannelRegistry) TryRegister(runID, projectID, workerID, assignmentID string, attempt int) (chan *workerv1.TaskResult, bool) {
	ch := make(chan *workerv1.TaskResult, 1)
	lock := r.lockForRun(runID)
	lock.Lock()
	defer lock.Unlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.channels[runID]; exists {
		return nil, false
	}
	r.channels[runID] = resultChannelEntry{
		ch:           ch,
		projectID:    projectID,
		workerID:     workerID,
		assignmentID: assignmentID,
		attempt:      attempt,
	}
	return ch, true
}

// Deregister removes the channel for the given run ID. Must be called by
// WorkerDispatch when the dispatch completes (deferred).
func (r *ResultChannelRegistry) Deregister(runID string) {
	lock := r.lockForRun(runID)
	lock.Lock()
	defer lock.Unlock()

	r.mu.Lock()
	delete(r.channels, runID)
	r.mu.Unlock()
}

// Send delivers a TaskResult to the channel for the given run ID, ONLY if the
// caller's project ID matches the project ID the channel was registered with.
// Returns true on successful delivery; false on missing channel, project
// mismatch, or already-buffered duplicate.
func (r *ResultChannelRegistry) Send(runID, projectID, workerID string, result *workerv1.TaskResult) bool {
	lock := r.lockForRun(runID)
	lock.Lock()
	defer lock.Unlock()

	r.mu.Lock()
	entry, ok := r.channels[runID]
	r.mu.Unlock()
	if !ok || !entry.matches(projectID, workerID, result) {
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

// SendAfterHandoff atomically reserves delivery to a waiting dispatcher, runs
// the caller's durable handoff, then publishes the result into the buffered
// channel before disconnect cleanup can observe the task as requeueable.
func (r *ResultChannelRegistry) SendAfterHandoff(
	runID, projectID, workerID string,
	result *workerv1.TaskResult,
	handoff func() (bool, error),
) (bool, error) {
	lock := r.lockForRun(runID)
	lock.Lock()
	defer lock.Unlock()

	r.mu.Lock()
	entry, ok := r.channels[runID]
	r.mu.Unlock()
	if !ok || !entry.matches(projectID, workerID, result) {
		return false, nil
	}
	if len(entry.ch) == cap(entry.ch) {
		return false, nil
	}

	handedOff, err := handoff()
	if err != nil || !handedOff {
		return false, err
	}

	select {
	case entry.ch <- result:
		return true, nil
	default:
		return false, nil
	}
}

func (r *ResultChannelRegistry) lockForRun(runID string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(runID))
	return &r.runLocks[h.Sum32()%uint32(len(r.runLocks))]
}

func (e resultChannelEntry) matches(projectID, workerID string, result *workerv1.TaskResult) bool {
	if result == nil {
		return false
	}
	return e.projectID == projectID &&
		e.workerID == workerID &&
		e.assignmentID != "" &&
		result.AssignmentId == e.assignmentID &&
		e.attempt > 0 &&
		int(result.Attempt) == e.attempt
}

// ErrNoWorkerAvailable is returned when no connected worker services the run's queue.
//
// Deprecated: use ErrNoWorkerForQueue.
var ErrNoWorkerAvailable = ErrNoWorkerForQueue

const invalidWorkerOutputError = "worker returned invalid output_json"

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
) (out any, err error) {
	started := time.Now()
	trace := newDispatchTrace(run, job)
	defer func() {
		if err == nil {
			recordGRPCDispatchE2E(ctx, started)
		}
		trace.finish(err)
	}()

	// Atomic pick + slot reservation under the registry write lock. Without
	// this, N concurrent dispatchers can all see SlotsAvailable=1 on the
	// same worker, all decrement, and oversubscribe a single-slot worker.
	workerID, sendCh, ok := d.registry.ReserveWorkerForQueue(run.ProjectID, job.Queue, job.EnvironmentID)
	trace.WorkerID = workerID
	if !ok {
		trace.Decision = "no_worker"
		return nil, ErrNoWorkerAvailable
	}
	trace.Decision = "worker_reserved"
	if sendCh == nil {
		// Defensive: a worker entry without a send channel should never be
		// pickable, but if one slipped through, release the reservation.
		d.registry.IncrementProjectSlots(run.ProjectID, workerID)
		trace.Result = "nil_send_channel"
		return nil, ErrNoWorkerAvailable
	}

	// Insert worker_tasks record so the reaper can pick it up if the
	// worker disconnects without reporting a result.
	attempt := run.Attempt
	if attempt <= 0 {
		attempt = 1
	}
	task := &domain.WorkerTask{
		ID:        uuid.Must(uuid.NewV7()).String(),
		WorkerID:  workerID,
		RunID:     run.ID,
		ProjectID: run.ProjectID,
		Attempt:   attempt,
		Status:    domain.WorkerTaskStatusAssigned,
	}
	if err := d.queries.CreateWorkerTask(ctx, task); err != nil {
		d.registry.IncrementProjectSlots(run.ProjectID, workerID)
		trace.TaskID = task.ID
		trace.Result = "task_record_failed"
		return nil, fmt.Errorf("worker dispatch: record task: %w", err)
	}
	trace.TaskID = task.ID

	// Register the result channel before sending the assignment so we can
	// never miss a TaskResult that arrives before we start waiting. The
	// channel is bound to run.ProjectID so any TaskResult arriving from a
	// worker authenticated to a different project is dropped on the floor.
	resultCh, registered := d.resultChannels.TryRegister(run.ID, run.ProjectID, workerID, task.ID, task.Attempt)
	if !registered {
		d.markWorkerTaskFailedAfterAbort(ctx, task.ID, run.ID)
		d.registry.IncrementProjectSlots(run.ProjectID, workerID)
		trace.Result = "result_channel_duplicate"
		return nil, ErrResultChannelAlreadyRegistered
	}
	defer d.resultChannels.Deregister(run.ID)

	// Build and send the TaskAssignment.
	assignment, err := d.buildAssignment(run, job, task.ID)
	if err != nil {
		d.markWorkerTaskFailedAfterAbort(ctx, task.ID, run.ID)
		d.registry.IncrementProjectSlots(run.ProjectID, workerID)
		return nil, err
	}
	msg := &workerv1.ServerMessage{
		Payload: &workerv1.ServerMessage_TaskAssignment{
			TaskAssignment: assignment,
		},
	}

	select {
	case sendCh <- msg:
		trace.Result = "assignment_queued"
		d.emitTaskRoutedAudit(ctx, run, job, workerID)
	case <-ctx.Done():
		d.markWorkerTaskFailedAfterAbort(ctx, task.ID, run.ID)
		d.registry.IncrementProjectSlots(run.ProjectID, workerID)
		trace.Result = "send_cancelled"
		return nil, ctx.Err()
	}

	// Wait for the TaskResult or context cancellation.
	select {
	case result, open := <-resultCh:
		d.registry.IncrementProjectSlots(run.ProjectID, workerID)
		if !open || result == nil {
			trace.Result = "result_channel_closed"
			return nil, fmt.Errorf("worker dispatch: result channel closed for run %s", run.ID)
		}
		if marked, err := d.markWorkerTaskResultReceived(ctx, task.ID, run.ID); err != nil {
			trace.Result = "task_result_mark_failed"
			return nil, err
		} else if !marked {
			trace.Result = "task_assignment_closed"
			return nil, fmt.Errorf("worker dispatch: task assignment closed before result for run %s", run.ID)
		}
		// Return as interface{} so callers in worker package don't need to
		// import workerv1. The task ID stays attached so the executor marks
		// worker_tasks terminal only after run result persistence succeeds.
		trace.Result = "result_received"
		return &WorkerTaskResult{TaskID: task.ID, Result: result}, nil

	case <-ctx.Done():
		// Best-effort cancellation: notify the worker.
		d.sendCancel(sendCh, run.ID)
		d.markWorkerTaskFailedAfterAbort(ctx, task.ID, run.ID)
		d.registry.IncrementProjectSlots(run.ProjectID, workerID)
		trace.Result = "wait_cancelled"
		return nil, ctx.Err()
	}
}

type dispatchTrace struct {
	Started   time.Time
	RunID     string
	JobID     string
	Queue     string
	ProjectID string
	WorkerID  string
	TaskID    string
	Decision  string
	Result    string
}

func newDispatchTrace(run *domain.JobRun, job *domain.Job) *dispatchTrace {
	trace := &dispatchTrace{Started: time.Now()}
	if run != nil {
		trace.RunID = run.ID
		trace.ProjectID = run.ProjectID
		trace.JobID = run.JobID
	}
	if job != nil {
		if trace.JobID == "" {
			trace.JobID = job.ID
		}
		trace.Queue = job.Queue
	}
	return trace
}

func (t *dispatchTrace) finish(err error) {
	if t == nil {
		return
	}
	logger := slog.Default()
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	result := t.Result
	if result == "" {
		if err != nil {
			result = "error"
		} else {
			result = "success"
		}
	}
	args := []any{
		"run_id", t.RunID,
		"job_id", t.JobID,
		"queue", t.Queue,
		"project_id", t.ProjectID,
		"worker_id", t.WorkerID,
		"task_id", t.TaskID,
		"decision", t.Decision,
		"result", result,
		"duration_ms", time.Since(t.Started).Milliseconds(),
	}
	if err != nil {
		args = append(args, "error", err)
	}
	logger.Debug("worker dispatch trace", args...)
}

func (d *WorkerDispatcher) emitTaskRoutedAudit(ctx context.Context, run *domain.JobRun, job *domain.Job, workerID string) {
	if d.queries == nil || run == nil || job == nil {
		return
	}
	details := map[string]any{
		"run_id":     run.ID,
		"worker_id":  workerID,
		"queue":      job.Queue,
		"project_id": run.ProjectID,
	}
	raw, err := json.Marshal(details)
	if err != nil {
		slog.Warn("worker dispatch: marshal task route audit failed", "run_id", run.ID, "error", err)
		return
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	ev := &domain.AuditEvent{
		ProjectID:     run.ProjectID,
		ActorID:       "system:worker-dispatcher",
		ActorType:     "system",
		Action:        domain.AuditActionWorkerTaskRouted,
		ResourceType:  "run",
		ResourceID:    run.ID,
		Details:       json.RawMessage(raw),
		SchemaVersion: domain.AuditEventSchemaVersionCurrent,
	}
	if err := d.queries.CreateAuditEvent(auditCtx, ev); err != nil {
		slog.Warn("worker dispatch: create task route audit failed",
			"run_id", run.ID,
			"worker_id", workerID,
			"error", err,
		)
	}
}

func (d *WorkerDispatcher) markWorkerTaskResultReceived(ctx context.Context, taskID, runID string) (bool, error) {
	if d.queries == nil || taskID == "" {
		return true, nil
	}
	markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	marked, err := d.queries.MarkWorkerTaskResultReceived(markCtx, taskID)
	if err != nil {
		return false, fmt.Errorf("worker dispatch: mark task result received for run %s: %w", runID, err)
	}
	return marked, nil
}

func (d *WorkerDispatcher) markWorkerTaskFailedAfterAbort(ctx context.Context, taskID, runID string) {
	if d.queries == nil || taskID == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := d.queries.UpdateWorkerTaskStatus(cleanupCtx, taskID, domain.WorkerTaskStatusFailed); err != nil {
		slog.Warn("worker dispatch: mark aborted task failed",
			"task_id", taskID,
			"run_id", runID,
			"error", err,
		)
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
func (d *WorkerDispatcher) buildAssignment(run *domain.JobRun, job *domain.Job, assignmentID string) (*workerv1.TaskAssignment, error) {
	attempt := run.Attempt
	if attempt <= 0 {
		attempt = 1
	}
	a := &workerv1.TaskAssignment{
		RunId:        run.ID,
		JobSlug:      job.Slug,
		Queue:        job.Queue,
		PayloadJson:  run.Payload,
		TimeoutSecs:  int32(job.TimeoutSecs), //nolint:gosec // TimeoutSecs is validated upstream to be non-negative and within range
		AssignmentId: assignmentID,
		Attempt:      int32(attempt),
	}

	// JWT run-token so the worker SDK can authenticate callbacks.
	if d.jwtSigningKey != "" {
		expiresAt := time.Now().Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
		if run.ExpiresAt != nil {
			expiresAt = *run.ExpiresAt
		}
		claims := struct {
			Attempt      int    `json:"attempt,omitempty"`
			AssignmentID string `json:"assignment_id,omitempty"`
			jwt.RegisteredClaims
		}{
			Attempt:      attempt,
			AssignmentID: assignmentID,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    domain.RunTokenIssuer,
				Subject:   run.ID,
				ExpiresAt: jwt.NewNumericDate(expiresAt),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := tok.SignedString([]byte(d.jwtSigningKey))
		if err != nil {
			// Do not dispatch a worker task with an empty run token: every SDK
			// callback would 401 and the run would hang in executing. Fail the
			// assignment so dispatch surfaces the error instead of swallowing it.
			return nil, fmt.Errorf("sign run token: %w", err)
		}
		a.RunTokenJwt = signed
	}

	// HMAC-SHA256 body+timestamp signing (same algorithm as worker.SignHTTPDispatch
	// in internal/worker/sign.go — reproduced here to avoid circular import).
	if job.EndpointSigningSecret != "" {
		signingSecret, err := straitcrypto.DecryptField(d.secretDecryptor, job.EndpointSigningSecret)
		if err != nil {
			return nil, fmt.Errorf("decrypt endpoint signing secret: %w", err)
		}
		ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
		a.HmacTimestamp = ts
		a.HmacSignature = dispatchHMAC(signingSecret, ts, run.Payload)
	}

	return a, nil
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
	r, ok := unwrapTaskResult(opaque)
	if !ok || r == nil {
		return ""
	}
	if taskResultOutputInvalid(r.Status, r.OutputJson) {
		return "failed"
	}
	return r.Status
}

// TaskResultError returns the error message from an opaque TaskResult.
func TaskResultError(opaque any) string {
	r, ok := unwrapTaskResult(opaque)
	if !ok || r == nil {
		return ""
	}
	if taskResultOutputInvalid(r.Status, r.OutputJson) {
		return invalidWorkerOutputError
	}
	return r.ErrorMessage
}

// TaskResultOutput returns output_json from an opaque TaskResult.
// A copy is returned so callers can safely retain it after the proto object is reused.
func TaskResultOutput(opaque any) json.RawMessage {
	r, ok := unwrapTaskResult(opaque)
	if !ok || r == nil || len(r.OutputJson) == 0 {
		return nil
	}
	if taskResultOutputInvalid(r.Status, r.OutputJson) {
		return nil
	}
	out := make([]byte, len(r.OutputJson))
	copy(out, r.OutputJson)
	return json.RawMessage(out)
}

func taskResultOutputInvalid(status string, output []byte) bool {
	return (status == "success" || status == "completed") && len(output) > 0 && !json.Valid(output)
}

func unwrapTaskResult(opaque any) (*workerv1.TaskResult, bool) {
	switch r := opaque.(type) {
	case *workerv1.TaskResult:
		return r, true
	case *WorkerTaskResult:
		if r == nil {
			return nil, false
		}
		return r.Result, true
	default:
		return nil, false
	}
}

func (d *WorkerDispatcher) CompleteWorkerTask(ctx context.Context, opaque any, status domain.WorkerTaskStatus) error {
	wrapped, ok := opaque.(*WorkerTaskResult)
	if !ok || wrapped == nil || wrapped.TaskID == "" || d.queries == nil {
		return nil
	}
	if !isTerminalWorkerTaskCompletionStatus(status) {
		return fmt.Errorf("worker dispatch: unsupported terminal worker task status %q", status)
	}
	return d.queries.UpdateWorkerTaskStatus(ctx, wrapped.TaskID, status)
}

func isTerminalWorkerTaskCompletionStatus(status domain.WorkerTaskStatus) bool {
	switch status {
	case domain.WorkerTaskStatusCompleted, domain.WorkerTaskStatusFailed:
		return true
	default:
		return false
	}
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
