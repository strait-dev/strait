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

	if reg.WorkerID == "" {
		return status.Error(codes.InvalidArgument, "worker_id must be non-empty")
	}

	// Reconcile in-flight tasks from the registration (reconnect recovery).
	// Passing workerID enables the adversarial ownership check.
	s.reconcileInFlightTasks(ctx, reg.WorkerID, projectID, reg.InFlightTasks)

	// Per-stream typed send channel for outbound ServerMessages.
	// The ConnectedWorker entry in the registry stores a send-only view of this
	// channel so that the dispatcher (and future cross-replica signals) can push
	// assignments without knowing the concrete message type.
	sendCh := make(chan *workerv1.ServerMessage, 32)

	// Register worker in the in-memory registry.
	cw := &ConnectedWorker{
		WorkerID:       reg.WorkerID,
		ProjectID:      projectID,
		APIKeyID:       apiKey.ID,
		Name:           reg.Name,
		Hostname:       reg.Hostname,
		SDKVersion:     reg.SDKVersion,
		SDKLanguage:    reg.SDKLanguage,
		Queues:         reg.Queues,
		SlotsTotal:     reg.SlotsTotal,
		SlotsAvailable: reg.SlotsAvailable,
		Status:         "active",
		SendCh:         nil, // populated below after sendCh is created
		revokeCh:       make(chan struct{}),
	}
	s.registry.Register(cw)

	// Upsert worker into DB immediately (don't wait for the next sync tick).
	s.dbUpsertWorker(ctx, cw)

	// Emit audit event.
	s.emitWorkerAudit(ctx, domain.AuditActionWorkerConnected, projectID, reg.WorkerID, map[string]interface{}{
		"worker_id": reg.WorkerID,
		"hostname":  reg.Hostname,
		"queues":    reg.Queues,
	})

	slog.Info("grpc worker registered",
		"worker_id", reg.WorkerID,
		"project_id", projectID,
		"hostname", reg.Hostname,
		"queues", reg.Queues,
		"slots_total", reg.SlotsTotal,
	)

	// Acknowledge registration.
	_ = stream.Send(&workerv1.ServerMessage{
		Payload: &workerv1.ServerMessage_Ack{
			Ack: &workerv1.Acknowledged{ID: reg.WorkerID},
		},
	})

	// Clean up on any exit path.
	defer func() {
		s.registry.Deregister(reg.WorkerID)
		s.emitWorkerAudit(ctx, domain.AuditActionWorkerDisconnected, projectID, reg.WorkerID, map[string]interface{}{
			"worker_id": reg.WorkerID,
		})
		slog.Info("grpc worker disconnected", "worker_id", reg.WorkerID, "project_id", projectID)
	}()

	// Subscribe to the cross-replica force-disconnect channel for this worker.
	// When DELETE /v1/workers/:id is called on any replica, it publishes to this
	// channel and the owning replica (which is running this goroutine) closes the stream.
	disconnectChannel := fmt.Sprintf("worker:disconnect:%s", reg.WorkerID)
	disconnectSub, subErr := s.pub.Subscribe(ctx, disconnectChannel)
	if subErr != nil {
		slog.Warn("grpc: failed to subscribe to disconnect channel",
			"worker_id", reg.WorkerID,
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
					"worker_id", reg.WorkerID,
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
					"worker_id", reg.WorkerID,
					"api_key_id", apiKey.ID,
					"project_id", projectID,
				)
				// Also close via registry so co-located streams for the same key are notified.
				s.registry.CloseByAPIKey(apiKey.ID)
				streamErr <- fmt.Errorf("api key revoked")
			case <-cw.revokeCh:
				// Triggered locally by registry.CloseByAPIKey from another goroutine.
				slog.Info("grpc worker api key revoked (local signal), closing stream",
					"worker_id", reg.WorkerID,
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
			if err := s.handleWorkerMessage(ctx, reg.WorkerID, projectID, msg); err != nil {
				slog.Warn("grpc handle worker message error",
					"worker_id", reg.WorkerID,
					"error", err,
				)
			}
		}
	})

	// Wait for first error.
	firstErr := <-streamErr
	close(sendCh)
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
		return s.handleLogLine(ctx, p.LogLine)
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

// handleHeartbeat updates the worker's last_seen_at in memory and periodically in DB.
func (s *workerService) handleHeartbeat(ctx context.Context, workerID string, hb *workerv1.Heartbeat) error {
	if hb == nil {
		return nil
	}
	// Update slot hint from worker (informational only — server is authoritative).
	// The DB heartbeat is handled by the dbSync loop; we just touch last_seen_at here.
	if err := s.queries.HeartbeatWorker(ctx, workerID); err != nil {
		slog.Warn("grpc heartbeat db update failed", "worker_id", workerID, "error", err)
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
	if tr == nil || tr.RunID == "" {
		return nil
	}

	// Route result to a waiting WorkerDispatch call if one exists.
	// The dispatch goroutine is responsible for slot accounting in that path.
	if s.resultChannels != nil && s.resultChannels.Send(tr.RunID, tr) {
		// Successfully delivered to the waiting dispatcher — it owns the rest.
		return nil
	}

	// No dispatcher is waiting (e.g. timed out or disconnected mid-flight).
	// Fall back: update the run status directly and restore the slot.
	s.registry.IncrementSlots(workerID)

	var newStatus domain.RunStatus
	switch tr.Status {
	case "success":
		newStatus = domain.StatusCompleted
	case "failed":
		newStatus = domain.StatusFailed
	default:
		newStatus = domain.StatusFailed
	}

	// Transition the run to its terminal state.
	finishedAt := time.Now()
	fields := map[string]any{"finished_at": finishedAt}
	if tr.ErrorMessage != "" {
		fields["error"] = tr.ErrorMessage
	}
	if err := s.queries.UpdateRunStatus(ctx, tr.RunID, domain.StatusExecuting, newStatus, fields); err != nil {
		slog.Warn("grpc task result: update run status failed",
			"run_id", tr.RunID,
			"status", newStatus,
			"error", err,
		)
	}

	// Publish result to the per-run pub/sub channel so SSE subscribers get notified.
	type runResultEvent struct {
		RunID  string `json:"run_id"`
		Status string `json:"status"`
	}
	payload, _ := json.Marshal(runResultEvent{RunID: tr.RunID, Status: string(newStatus)})
	if err := s.pub.Publish(ctx, fmt.Sprintf("run:%s", tr.RunID), payload); err != nil {
		slog.Warn("grpc task result: publish failed", "run_id", tr.RunID, "error", err)
	}

	return nil
}

// handleLogLine publishes a worker log line to the per-run pub/sub channel.
func (s *workerService) handleLogLine(ctx context.Context, ll *workerv1.LogLine) error {
	if ll == nil || ll.RunID == "" {
		return nil
	}
	type logLineEvent struct {
		RunID     string `json:"run_id"`
		Level     string `json:"level"`
		Message   string `json:"message"`
		Timestamp int64  `json:"timestamp_unix_ms"`
	}
	payload, _ := json.Marshal(logLineEvent{
		RunID:     ll.RunID,
		Level:     ll.Level,
		Message:   ll.Message,
		Timestamp: ll.TimestampUnixMS,
	})
	channel := fmt.Sprintf("worker:log:%s", ll.RunID)
	if err := s.pub.Publish(ctx, channel, payload); err != nil {
		slog.Warn("grpc log line publish failed", "run_id", ll.RunID, "error", err)
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
func (s *workerService) reconcileInFlightTasks(ctx context.Context, workerID, projectID string, tasks []*workerv1.InFlightTask) {
	for _, t := range tasks {
		if t == nil || t.RunID == "" {
			continue
		}

		// Adversarial guard: verify ownership via worker_tasks.
		taskRow, err := s.queries.GetWorkerTaskByRunID(ctx, workerID, t.RunID)
		if err != nil {
			slog.Warn("grpc reconcile: ownership lookup failed",
				"worker_id", workerID,
				"run_id", t.RunID,
				"error", err,
			)
			continue
		}
		if taskRow == nil {
			// No matching worker_tasks row — mismatch; reject.
			slog.Warn("grpc reconcile: rejecting in-flight task not owned by this worker",
				"worker_id", workerID,
				"run_id", t.RunID,
			)
			continue
		}

		switch t.Status {
		case "completed":
			reconcileFields := map[string]any{"finished_at": time.Now()}
			if err := s.queries.UpdateRunStatus(ctx, t.RunID, domain.StatusExecuting, domain.StatusCompleted, reconcileFields); err != nil {
				slog.Warn("grpc reconcile: mark completed failed",
					"run_id", t.RunID,
					"error", err,
				)
			}

		case "failed", "abandoned":
			// For failed/abandoned: attempt a retry if the job allows it,
			// otherwise mark as dead-letter.
			s.reconcileFailedTask(ctx, t)

		default:
			slog.Warn("grpc reconcile: unknown in-flight task status",
				"worker_id", workerID,
				"run_id", t.RunID,
				"status", t.Status,
			)
		}
	}
}

// reconcileFailedTask applies retry-or-fail logic for a failed/abandoned run
// reported during worker reconnection. It mirrors the retry scheduling in
// internal/worker/executor_handlers.go without importing that package.
func (s *workerService) reconcileFailedTask(ctx context.Context, t *workerv1.InFlightTask) {
	run, err := s.queries.GetRun(ctx, t.RunID)
	if err != nil || run == nil {
		slog.Warn("grpc reconcile: get run failed",
			"run_id", t.RunID,
			"error", err,
		)
		return
	}

	job, err := s.queries.GetJob(ctx, run.JobID)
	if err != nil || job == nil {
		slog.Warn("grpc reconcile: get job failed",
			"run_id", t.RunID,
			"job_id", run.JobID,
			"error", err,
		)
		// Fall back: mark failed without retry.
		s.reconcileMarkFailed(ctx, t.RunID, t.ErrorMessage)
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
		if err := s.queries.UpdateRunStatus(ctx, t.RunID, domain.StatusExecuting, domain.StatusQueued, fields); err != nil {
			slog.Warn("grpc reconcile: retry requeue failed",
				"run_id", t.RunID,
				"error", err,
			)
		} else {
			slog.Info("grpc reconcile: run requeued for retry",
				"run_id", t.RunID,
				"attempt", run.Attempt+1,
				"next_retry_at", retryAt,
			)
		}
		return
	}

	// Exhausted retries: mark dead-letter.
	s.reconcileMarkFailed(ctx, t.RunID, t.ErrorMessage)
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

// retryBackoffDuration returns an exponential backoff delay for a given attempt
// (1-indexed). Matches the default policy in internal/worker/backoff.go:
// delay = min(2^(attempt-1), 3600) seconds.
func retryBackoffDuration(attempt int) time.Duration {
	secs := 1 << (attempt - 1) // 2^(attempt-1)
	if secs > 3600 {
		secs = 3600
	}
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

// emitWorkerAudit writes an audit event for a worker lifecycle transition.
// Failures are logged but do not abort the caller.
func (s *workerService) emitWorkerAudit(ctx context.Context, action, projectID, workerID string, details map[string]interface{}) {
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

