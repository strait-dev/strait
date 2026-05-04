package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Resource bounds for incoming worker messages. Without these, a malicious or
// buggy worker can register with millions of queues / in-flight tasks /
// log lines and exhaust server memory or DB capacity.
const (
	maxWorkerIDLen     = 128
	maxQueuesPerWorker = 64
	maxInFlightTasks   = 256
	maxLogMessageBytes = 4096
	maxLogLevelBytes   = 32
	maxRunIDLen        = 128
	maxErrorMsgBytes   = 8192
	// maxSlotsPerWorker bounds the slots count a worker can advertise on
	// registration. PickWorkerForQueue ranks by SlotsAvailable, so an
	// unbounded value lets a buggy or malicious worker monopolize dispatch
	// for its project. 1024 leaves several orders of magnitude of headroom
	// over realistic SDK concurrency (4–32 typical).
	maxSlotsPerWorker = 1024
	// Bounds for unconstrained string fields a worker advertises on
	// registration. Without these a misbehaving SDK can register with
	// megabyte-scale Hostname/SDK metadata, bloating the in-memory registry,
	// the dbSync UPSERT, and any audit row that captures the registration.
	// Limits are generous against typical real values (POSIX HOST_NAME_MAX is
	// 255; SDK versions and language tokens are short identifiers).
	maxHostnameBytes    = 255
	maxSDKVersionBytes  = 64
	maxSDKLanguageBytes = 32
	maxNameBytes        = 128
)

// workerService implements workerv1.WorkerServiceServer.
type workerService struct {
	queries        *store.Queries
	pub            pubsub.Publisher
	registry       *ConnectionRegistry
	cfg            *config.Config
	resultChannels *ResultChannelRegistry
}

// StreamTasks is the bidirectional streaming RPC between the server and a worker SDK.
//
// Protocol:
//  1. Client sends WorkerRegistration as first message.
//  2. Server registers the worker and begins dispatching TaskAssignment messages.
//  3. Client sends Heartbeat periodically to refresh last_seen_at.
//  4. Client sends TaskResult when a run completes or fails.
//  5. Client sends LogLine for in-flight run logs.
//  6. On disconnect: server deregisters the worker and emits an audit event.
func (s *workerService) StreamTasks(stream workerv1.WorkerService_StreamTasksServer) error {
	ctx := stream.Context()

	// Authenticate the connecting worker via the Bearer API key in gRPC metadata.
	apiKey, err := resolveAPIKeyFromContext(ctx, s.queries)
	if err != nil {
		return err
	}
	ctx = withAPIKeyContext(ctx, apiKey)
	projectID := apiKey.ProjectID

	// Receive and validate the registration message.
	firstMsg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Internal, "recv registration: %v", err)
	}
	regPayload, ok := firstMsg.Payload.(*workerv1.WorkerMessage_Registration)
	if !ok || regPayload.Registration == nil {
		return status.Error(codes.InvalidArgument, "first message must be WorkerRegistration")
	}
	reg := regPayload.Registration

	if err := validateRegistration(reg); err != nil {
		return err
	}

	// Reject cross-project worker_id collisions. The in-memory registry
	// rejects same-id-different-api-key on this replica, but a separate
	// replica or a stale workers row could already own this id under a
	// different project. Without this check, the DB-side
	// `WHERE workers.project_id = EXCLUDED.project_id` upsert guard would
	// silently no-op the row sync, leaving the worker alive in memory but
	// invisible in the DB to its own project.
	if existingProject, ok, err := s.queries.GetWorkerProjectByID(ctx, reg.WorkerId); err != nil {
		slog.Warn("grpc registration: worker project lookup failed",
			"worker_id", reg.WorkerId, "error", err)
		return status.Errorf(codes.Internal, "worker registration: lookup failed")
	} else if ok && existingProject != projectID {
		return status.Errorf(codes.AlreadyExists,
			"worker_id %q already registered under a different project", reg.WorkerId)
	}

	// Per-stream typed send channel for outbound ServerMessages.
	// The dispatcher pushes TaskAssignment / CancelTask messages here; the send
	// loop below drains the channel and writes each message to the gRPC stream.
	sendCh := make(chan *workerv1.ServerMessage, 32)

	// Register worker in the in-memory registry. SendCh is assigned BEFORE
	// Register so any concurrent dispatch on this replica sees a usable channel.
	cw := &ConnectedWorker{
		WorkerID:       reg.WorkerId,
		ProjectID:      projectID,
		APIKeyID:       apiKey.ID,
		Name:           reg.Name,
		Hostname:       reg.Hostname,
		SDKVersion:     reg.SdkVersion,
		SDKLanguage:    reg.SdkLanguage,
		Queues:         reg.Queues,
		SlotsTotal:     reg.SlotsTotal,
		SlotsAvailable: reg.SlotsAvailable,
		Status:         "active",
		SendCh:         sendCh,
		revokeCh:       make(chan struct{}),
	}
	if err := s.registry.Register(cw); err != nil {
		return status.Errorf(codes.AlreadyExists, "register worker: %v", err)
	}

	// Reconcile in-flight tasks from the registration (reconnect recovery).
	// Passing workerID enables the adversarial ownership check.
	s.reconcileInFlightTasks(ctx, reg.WorkerId, projectID, reg.InFlightTasks)

	// Upsert worker into DB immediately (don't wait for the next sync tick).
	s.dbUpsertWorker(ctx, cw)

	// Emit audit event.
	s.emitWorkerAudit(ctx, domain.AuditActionWorkerConnected, projectID, reg.WorkerId, map[string]any{
		"worker_id": reg.WorkerId,
		"hostname":  reg.Hostname,
		"queues":    reg.Queues,
	})

	slog.Info("grpc worker registered",
		"worker_id", reg.WorkerId,
		"project_id", projectID,
		"hostname", reg.Hostname,
		"queues", reg.Queues,
		"slots_total", reg.SlotsTotal,
	)

	// Acknowledge registration.
	_ = stream.Send(&workerv1.ServerMessage{
		Payload: &workerv1.ServerMessage_Ack{
			Ack: &workerv1.Acknowledged{Id: reg.WorkerId},
		},
	})

	// Clean up on any exit path. Pass the per-registration token so a stale
	// goroutine cannot evict a live replacement that registered under the
	// same WorkerID after a reconnect race.
	myToken := cw.regToken
	defer func() {
		s.registry.Deregister(reg.WorkerId, myToken)
		s.finalizeDisconnect(projectID, reg.WorkerId)
	}()

	// Subscribe to the cross-replica force-disconnect channel for this worker.
	// When DELETE /v1/workers/:id is called on any replica, it publishes to this
	// channel and the owning replica (which is running this goroutine) closes the stream.
	disconnectChannel := fmt.Sprintf("worker:disconnect:%s", reg.WorkerId)
	disconnectSub, subErr := s.pub.Subscribe(ctx, disconnectChannel)
	if subErr != nil {
		slog.Warn("grpc: failed to subscribe to disconnect channel",
			"worker_id", reg.WorkerId,
			"error", subErr,
		)
	}

	// Subscribe to the API key revocation channel.
	// When POST /v1/api-keys/:id/revoke succeeds, it publishes to this channel
	// so every stream authenticated with that key closes across all replicas.
	var revokeKeySub *pubsub.Subscription
	if apiKey.ID != "" {
		revokeChannel := fmt.Sprintf("apikey:revoked:%s", apiKey.ID)
		revokeKeySub, _ = s.pub.Subscribe(ctx, revokeChannel)
	}

	// Run recv and send loops concurrently. If either exits, the stream closes.
	var wg conc.WaitGroup
	goroutineCount := 2
	if disconnectSub != nil {
		goroutineCount++
	}
	if revokeKeySub != nil {
		goroutineCount++
	}
	streamErr := make(chan error, goroutineCount)

	// Disconnect signal listener: closes the stream when a force-disconnect is published.
	if disconnectSub != nil {
		wg.Go(func() {
			defer disconnectSub.Close()
			select {
			case <-ctx.Done():
				streamErr <- nil
			case <-disconnectSub.Ch:
				slog.Info("grpc worker force-disconnect received",
					"worker_id", reg.WorkerId,
					"project_id", projectID,
				)
				streamErr <- fmt.Errorf("force disconnected by API request")
			}
		})
	}

	// API key revocation listener: closes the stream when the authenticating key is revoked.
	if revokeKeySub != nil {
		wg.Go(func() {
			defer revokeKeySub.Close()
			select {
			case <-ctx.Done():
				streamErr <- nil
			case <-revokeKeySub.Ch:
				slog.Info("grpc worker api key revoked, closing stream",
					"worker_id", reg.WorkerId,
					"api_key_id", apiKey.ID,
					"project_id", projectID,
				)
				// Also close via registry so co-located streams for the same key are notified.
				s.registry.CloseByAPIKey(apiKey.ID)
				streamErr <- fmt.Errorf("api key revoked")
			case <-cw.revokeCh:
				// Triggered locally by registry.CloseByAPIKey from another goroutine.
				slog.Info("grpc worker api key revoked (local signal), closing stream",
					"worker_id", reg.WorkerId,
					"api_key_id", apiKey.ID,
				)
				streamErr <- fmt.Errorf("api key revoked")
			}
		})
	}

	// Send loop: drain sendCh and write to the stream.
	wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				streamErr <- nil
				return
			case msg, open := <-sendCh:
				if !open {
					streamErr <- nil
					return
				}
				if err := stream.Send(msg); err != nil {
					streamErr <- fmt.Errorf("send: %w", err)
					return
				}
			}
		}
	})

	// Recv loop: process incoming worker messages.
	wg.Go(func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				streamErr <- err
				return
			}
			if err := s.handleWorkerMessage(ctx, reg.WorkerId, projectID, msg); err != nil {
				slog.Warn("grpc handle worker message error",
					"worker_id", reg.WorkerId,
					"error", err,
				)
			}
		}
	})

	// Wait for first error. We deregister synchronously *before* wg.Wait so
	// no new ReserveWorkerForQueue call can hand out our sendCh. We do NOT
	// close(sendCh): a concurrent WorkerDispatch that picked us before the
	// Deregister still holds a reference and would panic on send-to-closed.
	// Stale, in-flight sends fill the 32-slot buffer and then unblock when
	// the dispatcher's ctx times out; sendCh is GC'd once the last reference
	// drops. The deferred Deregister below remains as a safety net for
	// early-exit paths and is a no-op once this synchronous call removes
	// the entry.
	firstErr := <-streamErr
	s.registry.Deregister(reg.WorkerId, myToken)
	wg.Wait()
	return firstErr
}

// handleWorkerMessage dispatches an incoming WorkerMessage to the appropriate handler.
func (s *workerService) handleWorkerMessage(ctx context.Context, workerID, projectID string, msg *workerv1.WorkerMessage) error {
	switch p := msg.Payload.(type) {
	case *workerv1.WorkerMessage_Heartbeat:
		return s.handleHeartbeat(ctx, workerID, p.Heartbeat)
	case *workerv1.WorkerMessage_TaskResult:
		return s.handleTaskResult(ctx, workerID, projectID, p.TaskResult)
	case *workerv1.WorkerMessage_LogLine:
		return s.handleLogLine(ctx, workerID, p.LogLine)
	case *workerv1.WorkerMessage_Ack:
		// No-op: acknowledgements are fire-and-forget.
		return nil
	case *workerv1.WorkerMessage_Registration:
		// Re-registration on an established stream is ignored (handled at connect).
		return nil
	default:
		return nil
	}
}

// handleHeartbeat is a no-op on the DB. last_seen_at is refreshed by the
// dbSync loop (RegisterWorker UPSERT, every WORKER_DB_SYNC_INTERVAL ≈ 10s),
// which is well inside the WORKER_HEARTBEAT_TIMEOUT sweep window (≈ 30s).
// Writing on every heartbeat caused N×workers DB writes per HeartbeatInterval
// without changing observability — the dbSync row already carries the same
// timestamp. The slot hint in hb is informational; the server is
// authoritative on slot accounting via Increment/DecrementSlots.
func (s *workerService) handleHeartbeat(_ context.Context, _ string, hb *workerv1.Heartbeat) error {
	if hb == nil {
		return nil
	}
	return nil
}

// handleTaskResult reconciles a completed/failed run from the worker.
// If a WorkerDispatch call is waiting on this run, the result is routed via
// the ResultChannelRegistry so the dispatch goroutine can handle terminal
// state transitions (status update, cost recording). If no channel is
// registered (e.g. the dispatcher timed out), this method falls back to
// updating the run status directly.
func (s *workerService) handleTaskResult(ctx context.Context, workerID, projectID string, tr *workerv1.TaskResult) error {
	if tr == nil || tr.RunId == "" {
		return nil
	}
	// Bound RunId so a malicious worker can't use it as a pubsub-channel
	// amplifier or DB-key blow-up vector.
	if len(tr.RunId) > maxRunIDLen {
		slog.Warn("grpc task result: run_id exceeds bound — rejecting",
			"worker_id", workerID, "run_id_len", len(tr.RunId))
		return nil
	}
	// Cap error message so a worker can't bloat DB rows or page logs.
	if len(tr.ErrorMessage) > maxErrorMsgBytes {
		tr.ErrorMessage = tr.ErrorMessage[:maxErrorMsgBytes]
	}

	// Route result to a waiting WorkerDispatch call if one exists.
	// The dispatch goroutine is responsible for slot accounting in that path.
	// The result channel is project-scoped so a worker authenticated to a
	// different project cannot deliver a forged TaskResult into another
	// project's dispatch goroutine: Send drops the message on project mismatch.
	if s.resultChannels != nil && s.resultChannels.Send(tr.RunId, projectID, tr) {
		// Successfully delivered to the waiting dispatcher — it owns the rest.
		return nil
	}

	// No dispatcher is waiting (e.g. timed out or disconnected mid-flight)
	// OR the message was dropped above due to a project mismatch.
	// Adversarial guard: confirm the run belongs to this worker's project before
	// touching status. Without this check, a worker authenticated to project A
	// could mark runs in project B if it knew (or guessed) the run ID.
	run, err := s.queries.GetRun(ctx, tr.RunId)
	if err != nil || run == nil {
		slog.Warn("grpc task result: get run failed",
			"worker_id", workerID, "run_id", tr.RunId, "error", err)
		return nil
	}
	if run.ProjectID != projectID {
		slog.Warn("grpc task result: project mismatch — rejecting",
			"worker_id", workerID, "run_id", tr.RunId,
			"worker_project", projectID, "run_project", run.ProjectID)
		return nil
	}

	// Ownership guard: confirm the worker_tasks row exists and belongs to this
	// worker. This mirrors handleLogLine so a worker cannot mark runs it was
	// never assigned. The row also gives us the task ID needed to drive the
	// worker_tasks transition below.
	taskRow, taskErr := s.queries.GetWorkerTaskByRunID(ctx, workerID, tr.RunId)
	if taskErr != nil {
		slog.Warn("grpc task result fallback: ownership lookup failed",
			"worker_id", workerID, "run_id", tr.RunId, "error", taskErr)
		return nil
	}
	if taskRow == nil {
		slog.Warn("grpc task result fallback: rejecting — run not assigned to this worker",
			"worker_id", workerID, "run_id", tr.RunId)
		return nil
	}

	// Fall back: update the run status directly. Do NOT restore the slot
	// here — if a WorkerDispatch goroutine ever held this run, it has
	// already restored the slot on its ctx.Done() / result branch
	// (see dispatch.go IncrementSlots sites). If no dispatcher ever held
	// the run on this replica (e.g. cross-replica handoff, in-flight
	// reconnect), no slot was decremented here, so there is nothing to
	// credit. Calling IncrementSlots in this path produced an over-credit
	// when a late result arrived after the dispatcher's ctx.Done()
	// already restored the slot, letting the worker monopolize dispatch.

	var newStatus domain.RunStatus
	var newTaskStatus domain.WorkerTaskStatus
	switch tr.Status {
	case "success":
		newStatus = domain.StatusCompleted
		newTaskStatus = domain.WorkerTaskStatusCompleted
	case "failed":
		newStatus = domain.StatusFailed
		newTaskStatus = domain.WorkerTaskStatusFailed
	default:
		newStatus = domain.StatusFailed
		newTaskStatus = domain.WorkerTaskStatusFailed
	}

	// Transition the run to its terminal state.
	finishedAt := time.Now()
	fields := map[string]any{"finished_at": finishedAt}
	if tr.ErrorMessage != "" {
		fields["error"] = tr.ErrorMessage
	}
	if err := s.queries.UpdateRunStatus(ctx, tr.RunId, domain.StatusExecuting, newStatus, fields); err != nil {
		slog.Warn("grpc task result: update run status failed",
			"run_id", tr.RunId,
			"status", newStatus,
			"error", err,
		)
	}

	// Transition the worker_tasks row to its terminal state. The normal dispatch
	// path (dispatch.go) does this when the result arrives in time; the fallback
	// must do it too so a late TaskResult doesn't leave the row stuck in
	// "assigned" forever. UpdateWorkerTaskStatus is idempotent — safe to call
	// even if a concurrent normal-path update already wrote the same value.
	if err := s.queries.UpdateWorkerTaskStatus(ctx, taskRow.ID, newTaskStatus); err != nil {
		slog.Warn("grpc task result fallback: update worker_task status failed",
			"task_id", taskRow.ID,
			"run_id", tr.RunId,
			"status", newTaskStatus,
			"error", err,
		)
	}

	// Publish result to the per-run pub/sub channel so SSE subscribers get notified.
	type runResultEvent struct {
		RunID  string `json:"run_id"`
		Status string `json:"status"`
	}
	payload, _ := json.Marshal(runResultEvent{RunID: tr.RunId, Status: string(newStatus)})
	if err := s.pub.Publish(ctx, fmt.Sprintf("run:%s", tr.RunId), payload); err != nil {
		slog.Warn("grpc task result: publish failed", "run_id", tr.RunId, "error", err)
	}

	return nil
}

// handleLogLine publishes a worker log line to the per-run pub/sub channel.
//
// Adversarial guard: the worker may only emit logs for runs assigned to it
// via worker_tasks (the same row written by WorkerDispatch). Without this
// check, a worker authenticated to project A could publish forged log lines
// to any run in any project — visible via the SSE log stream.
func (s *workerService) handleLogLine(ctx context.Context, workerID string, ll *workerv1.LogLine) error {
	if ll == nil || ll.RunId == "" {
		return nil
	}
	if len(ll.RunId) > maxRunIDLen {
		slog.Warn("grpc log line: run_id exceeds bound — rejecting",
			"worker_id", workerID, "run_id_len", len(ll.RunId))
		return nil
	}
	taskRow, err := s.queries.GetWorkerTaskByRunID(ctx, workerID, ll.RunId)
	if err != nil {
		slog.Warn("grpc log line: ownership lookup failed",
			"worker_id", workerID, "run_id", ll.RunId, "error", err)
		return nil
	}
	if taskRow == nil {
		slog.Warn("grpc log line: rejecting — run not assigned to this worker",
			"worker_id", workerID, "run_id", ll.RunId)
		return nil
	}
	msg := ll.Message
	if len(msg) > maxLogMessageBytes {
		msg = msg[:maxLogMessageBytes]
	}
	level := ll.Level
	if len(level) > maxLogLevelBytes {
		level = level[:maxLogLevelBytes]
	}
	type logLineEvent struct {
		RunID     string `json:"run_id"`
		Level     string `json:"level"`
		Message   string `json:"message"`
		Timestamp int64  `json:"timestamp_unix_ms"`
	}
	payload, _ := json.Marshal(logLineEvent{
		RunID:     ll.RunId,
		Level:     level,
		Message:   msg,
		Timestamp: ll.TimestampUnixMs,
	})
	channel := fmt.Sprintf("worker:log:%s", ll.RunId)
	if err := s.pub.Publish(ctx, channel, payload); err != nil {
		slog.Warn("grpc log line publish failed", "run_id", ll.RunId, "error", err)
	}
	return nil
}

// reconcileInFlightTasks handles runs that the worker was executing at the time
// of (re)connection. It applies the correct terminal transition per status and,
// for failed/abandoned runs, schedules a retry per the job's retry policy
// (mirroring the executor's handleFailure path).
//
// Adversarial guard: before reconciling, the run is validated against
// worker_tasks to confirm it was actually assigned to this worker. Mismatches
// are logged and skipped — this prevents a malicious or buggy worker from
// marking runs it doesn't own.
func (s *workerService) reconcileInFlightTasks(ctx context.Context, workerID, _ string, tasks []*workerv1.InFlightTask) {
	for _, t := range tasks {
		if t == nil || t.RunId == "" {
			continue
		}
		if len(t.RunId) > maxRunIDLen {
			slog.Warn("grpc reconcile: run_id exceeds bound — skipping",
				"worker_id", workerID, "run_id_len", len(t.RunId))
			continue
		}
		if len(t.ErrorMessage) > maxErrorMsgBytes {
			t.ErrorMessage = t.ErrorMessage[:maxErrorMsgBytes]
		}

		// Adversarial guard: verify ownership via worker_tasks.
		taskRow, err := s.queries.GetWorkerTaskByRunID(ctx, workerID, t.RunId)
		if err != nil {
			slog.Warn("grpc reconcile: ownership lookup failed",
				"worker_id", workerID,
				"run_id", t.RunId,
				"error", err,
			)
			continue
		}
		if taskRow == nil {
			// No matching worker_tasks row — mismatch; reject.
			slog.Warn("grpc reconcile: rejecting in-flight task not owned by this worker",
				"worker_id", workerID,
				"run_id", t.RunId,
			)
			continue
		}

		switch t.Status {
		case "completed":
			reconcileFields := map[string]any{"finished_at": time.Now()}
			if err := s.queries.UpdateRunStatus(ctx, t.RunId, domain.StatusExecuting, domain.StatusCompleted, reconcileFields); err != nil {
				slog.Warn("grpc reconcile: mark completed failed",
					"run_id", t.RunId,
					"error", err,
				)
			}
			if err := s.queries.UpdateWorkerTaskStatus(ctx, taskRow.ID, domain.WorkerTaskStatusCompleted); err != nil {
				slog.Warn("grpc reconcile: update worker_task status failed",
					"task_id", taskRow.ID,
					"run_id", t.RunId,
					"error", err,
				)
			}

		case "failed", "abandoned":
			// For failed/abandoned: attempt a retry if the job allows it,
			// otherwise mark as dead-letter.
			s.reconcileFailedTask(ctx, t)
			// Whether the run gets requeued or dead-lettered, the worker_task
			// row that recorded this assignment is done — mark it failed so it
			// doesn't linger in "assigned" forever.
			if err := s.queries.UpdateWorkerTaskStatus(ctx, taskRow.ID, domain.WorkerTaskStatusFailed); err != nil {
				slog.Warn("grpc reconcile: update worker_task status failed",
					"task_id", taskRow.ID,
					"run_id", t.RunId,
					"error", err,
				)
			}

		default:
			slog.Warn("grpc reconcile: unknown in-flight task status",
				"worker_id", workerID,
				"run_id", t.RunId,
				"status", t.Status,
			)
		}
	}
}

// reconcileFailedTask applies retry-or-fail logic for a failed/abandoned run
// reported during worker reconnection. It mirrors the retry scheduling in
// internal/worker/executor_handlers.go without importing that package.
func (s *workerService) reconcileFailedTask(ctx context.Context, t *workerv1.InFlightTask) {
	run, err := s.queries.GetRun(ctx, t.RunId)
	if err != nil || run == nil {
		slog.Warn("grpc reconcile: get run failed",
			"run_id", t.RunId,
			"error", err,
		)
		return
	}

	job, err := s.queries.GetJob(ctx, run.JobID)
	if err != nil || job == nil {
		slog.Warn("grpc reconcile: get job failed",
			"run_id", t.RunId,
			"job_id", run.JobID,
			"error", err,
		)
		// Fall back: mark failed without retry.
		s.reconcileMarkFailed(ctx, t.RunId, t.ErrorMessage)
		return
	}

	// Determine whether another attempt is allowed.
	maxAttempts := job.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	if run.Attempt < maxAttempts {
		// Schedule retry: increment attempt, compute next_retry_at, requeue.
		retryAt := time.Now().Add(retryBackoffDuration(run.Attempt))
		fields := map[string]any{
			"attempt":       run.Attempt + 1,
			"next_retry_at": retryAt,
			"started_at":    nil,
			"finished_at":   nil,
		}
		if t.ErrorMessage != "" {
			fields["error"] = t.ErrorMessage
		}
		if err := s.queries.UpdateRunStatus(ctx, t.RunId, domain.StatusExecuting, domain.StatusQueued, fields); err != nil {
			slog.Warn("grpc reconcile: retry requeue failed",
				"run_id", t.RunId,
				"error", err,
			)
		} else {
			slog.Info("grpc reconcile: run requeued for retry",
				"run_id", t.RunId,
				"attempt", run.Attempt+1,
				"next_retry_at", retryAt,
			)
		}
		return
	}

	// Exhausted retries: mark dead-letter.
	s.reconcileMarkFailed(ctx, t.RunId, t.ErrorMessage)
}

// reconcileMarkFailed transitions a run to StatusDeadLetter.
func (s *workerService) reconcileMarkFailed(ctx context.Context, runID, errMsg string) {
	fields := map[string]any{"finished_at": time.Now()}
	if errMsg != "" {
		fields["error"] = errMsg
	}
	if err := s.queries.UpdateRunStatus(ctx, runID, domain.StatusExecuting, domain.StatusDeadLetter, fields); err != nil {
		slog.Warn("grpc reconcile: mark failed",
			"run_id", runID,
			"error", err,
		)
	}
}

// validateRegistration enforces the resource bounds and slot-count sanity
// checks on an incoming WorkerRegistration. Returning a typed gRPC status
// error (codes.InvalidArgument) lets the stream handler reject malformed
// registrations without doing any state mutation. Extracted as a pure
// function so it is exhaustively unit-testable.
//
// Slot-count checks defend the dispatcher: PickWorkerForQueue ranks
// workers by SlotsAvailable, so a worker advertising an oversized or
// negative slot count could either monopolize dispatch or wedge it. We
// trust the worker to report its own concurrency, but only within bounds
// the server can enforce.
func validateRegistration(reg *workerv1.WorkerRegistration) error {
	if reg == nil {
		return status.Error(codes.InvalidArgument, "registration must not be nil")
	}
	if reg.WorkerId == "" {
		return status.Error(codes.InvalidArgument, "worker_id must be non-empty")
	}
	if len(reg.WorkerId) > maxWorkerIDLen {
		return status.Errorf(codes.InvalidArgument, "worker_id exceeds %d bytes", maxWorkerIDLen)
	}
	if len(reg.Queues) > maxQueuesPerWorker {
		return status.Errorf(codes.InvalidArgument, "too many queues: max %d", maxQueuesPerWorker)
	}
	if len(reg.InFlightTasks) > maxInFlightTasks {
		return status.Errorf(codes.InvalidArgument, "too many in-flight tasks: max %d", maxInFlightTasks)
	}
	if reg.SlotsTotal < 0 {
		return status.Errorf(codes.InvalidArgument, "slots_total must be non-negative, got %d", reg.SlotsTotal)
	}
	if reg.SlotsTotal > maxSlotsPerWorker {
		return status.Errorf(codes.InvalidArgument, "slots_total exceeds %d", maxSlotsPerWorker)
	}
	if reg.SlotsAvailable < 0 {
		return status.Errorf(codes.InvalidArgument, "slots_available must be non-negative, got %d", reg.SlotsAvailable)
	}
	if reg.SlotsAvailable > reg.SlotsTotal {
		return status.Errorf(codes.InvalidArgument, "slots_available (%d) exceeds slots_total (%d)", reg.SlotsAvailable, reg.SlotsTotal)
	}
	if len(reg.Hostname) > maxHostnameBytes {
		return status.Errorf(codes.InvalidArgument, "hostname exceeds %d bytes", maxHostnameBytes)
	}
	if len(reg.SdkVersion) > maxSDKVersionBytes {
		return status.Errorf(codes.InvalidArgument, "sdk_version exceeds %d bytes", maxSDKVersionBytes)
	}
	if len(reg.SdkLanguage) > maxSDKLanguageBytes {
		return status.Errorf(codes.InvalidArgument, "sdk_language exceeds %d bytes", maxSDKLanguageBytes)
	}
	if len(reg.Name) > maxNameBytes {
		return status.Errorf(codes.InvalidArgument, "name exceeds %d bytes", maxNameBytes)
	}
	return nil
}

// retryBackoffDuration returns an exponential backoff delay for a given attempt
// (1-indexed). Matches the default policy in internal/worker/backoff.go:
// delay = min(2^(attempt-1), 3600) seconds.
func retryBackoffDuration(attempt int) time.Duration {
	secs := min(1<<(attempt-1), 3600) // 2^(attempt-1), capped at 3600
	return time.Duration(secs) * time.Second
}

// dbUpsertWorker immediately upserts the worker into the DB on registration,
// without waiting for the next dbSync tick.
func (s *workerService) dbUpsertWorker(ctx context.Context, cw *ConnectedWorker) {
	queueName := ""
	if len(cw.Queues) > 0 {
		queueName = cw.Queues[0]
	}
	dw := &domain.Worker{
		ID:        cw.WorkerID,
		ProjectID: cw.ProjectID,
		QueueName: queueName,
		Hostname:  cw.Hostname,
		Version:   cw.SDKVersion,
		Status:    domain.WorkerStatusActive,
	}
	if err := s.queries.RegisterWorker(ctx, dw); err != nil {
		slog.Warn("grpc: immediate db upsert on registration failed",
			"worker_id", cw.WorkerID,
			"error", err,
		)
	}
}

// finalizeDisconnect runs the post-stream cleanup writes: mark the worker
// offline in the workers table, then emit the disconnect audit event.
//
// The stream's ctx is cancelled by the time the deferred cleanup fires (that
// cancellation is precisely how we exit), so any DB call using it would fail
// with context canceled. We allocate a fresh background context with a short
// timeout so the offline transition and audit row still land — without this,
// the workers row stays in `active` forever after a clean disconnect and the
// audit log is missing the disconnect event entirely.
func (s *workerService) finalizeDisconnect(projectID, workerID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.queries.SetWorkerStatus(ctx, workerID, domain.WorkerStatusOffline); err != nil {
		slog.Warn("grpc worker disconnect: failed to mark offline",
			"worker_id", workerID, "error", err)
	}
	s.emitWorkerAudit(ctx, domain.AuditActionWorkerDisconnected, projectID, workerID, map[string]any{
		"worker_id": workerID,
	})
	slog.Info("grpc worker disconnected", "worker_id", workerID, "project_id", projectID)
}

// emitWorkerAudit writes an audit event for a worker lifecycle transition.
// Failures are logged but do not abort the caller.
func (s *workerService) emitWorkerAudit(ctx context.Context, action, projectID, workerID string, details map[string]any) {
	raw, err := json.Marshal(details)
	if err != nil {
		slog.Warn("grpc audit: marshal details failed", "error", err)
		return
	}
	ev := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "worker:" + workerID,
		ActorType:    "worker",
		Action:       action,
		ResourceType: "worker",
		ResourceID:   workerID,
		Details:      json.RawMessage(raw),
		CreatedAt:    time.Now(),
	}
	if err := s.queries.CreateAuditEvent(ctx, ev); err != nil {
		slog.Warn("grpc audit: create event failed",
			"action", action,
			"worker_id", workerID,
			"error", err,
		)
	}
}
