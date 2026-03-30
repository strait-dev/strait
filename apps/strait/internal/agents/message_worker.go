package agents

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"strait/internal/domain"
)

// MessageWorkerStore defines the store methods needed by the message delivery worker.
type MessageWorkerStore interface {
	ListPendingAgentMessages(ctx context.Context, limit int) ([]domain.AgentMessage, error)
	UpdateAgentMessageStatus(ctx context.Context, id string, status domain.AgentMessageStatus, fields map[string]any) error
	GetAgent(ctx context.Context, id string) (*domain.Agent, error)
	GetLatestAgentDeployment(ctx context.Context, agentID string) (*domain.AgentDeployment, error)
}

// MessageWorkerDeps holds dependencies for the message delivery worker.
type MessageWorkerDeps struct {
	Store        MessageWorkerStore
	AgentSvc     Service
	PollInterval time.Duration
	BatchSize    int
	Clock        func() time.Time
}

// MessageWorker polls for pending agent messages and dispatches them as agent runs.
type MessageWorker struct {
	store        MessageWorkerStore
	agentSvc     Service
	pollInterval time.Duration
	batchSize    int
	now          func() time.Time
	stop         chan struct{}
	done         chan struct{}
	stopOnce     sync.Once
	running      atomic.Bool
}

// NewMessageWorker creates a new agent message delivery worker.
func NewMessageWorker(deps MessageWorkerDeps) *MessageWorker {
	interval := deps.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	batchSize := deps.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	clock := deps.Clock
	if clock == nil {
		clock = time.Now
	}
	return &MessageWorker{
		store:        deps.Store,
		agentSvc:     deps.AgentSvc,
		pollInterval: interval,
		batchSize:    batchSize,
		now:          clock,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// Start begins the background polling loop.
func (w *MessageWorker) Start(ctx context.Context) {
	if w == nil || !w.running.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(w.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-w.stop:
				return
			case <-ticker.C:
				w.poll(ctx)
			}
		}
	}()
}

// Stop signals the worker to stop and waits for it to finish.
func (w *MessageWorker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		close(w.stop)
	})
	<-w.done
}

func (w *MessageWorker) poll(ctx context.Context) {
	messages, err := w.store.ListPendingAgentMessages(ctx, w.batchSize)
	if err != nil {
		slog.Error("agent message worker: list pending", "error", err)
		return
	}

	for _, msg := range messages {
		msgCopy := msg
		w.deliver(ctx, &msgCopy)
	}
}

func (w *MessageWorker) deliver(ctx context.Context, msg *domain.AgentMessage) {
	agent, err := w.store.GetAgent(ctx, msg.TargetAgentID)
	if err != nil {
		w.markFailed(ctx, msg.ID, fmt.Sprintf("target agent not found: %v", err))
		return
	}

	deployment, err := w.store.GetLatestAgentDeployment(ctx, agent.ID)
	if err != nil || deployment == nil || deployment.Status != domain.AgentDeploymentStatusDeployed {
		w.markFailed(ctx, msg.ID, "target agent has no active deployment")
		return
	}

	// Build payload with message metadata.
	payload := mustJSON(map[string]any{
		"_message_id":    msg.ID,
		"_chain_id":      msg.ChainID,
		"_chain_depth":   msg.ChainDepth,
		"_source_agent":  msg.SourceAgentID,
		"_source_run_id": msg.SourceRunID,
		"payload":        msg.Payload,
	})

	_, runErr := w.agentSvc.RunAgent(ctx, RunAgentRequest{
		ProjectID: msg.ProjectID,
		AgentID:   msg.TargetAgentID,
		Payload:   payload,
		Actor:     "system:message_worker",
	})
	if runErr != nil {
		w.markFailed(ctx, msg.ID, fmt.Sprintf("dispatch failed: %v", runErr))
		return
	}

	now := w.now().UTC()
	if updateErr := w.store.UpdateAgentMessageStatus(ctx, msg.ID, domain.AgentMessageDelivered, map[string]any{
		"delivered_at": now,
	}); updateErr != nil {
		slog.Error("agent message worker: update delivered", "msg_id", msg.ID, "error", updateErr)
	}
}

func (w *MessageWorker) markFailed(ctx context.Context, msgID, errMsg string) {
	if updateErr := w.store.UpdateAgentMessageStatus(ctx, msgID, domain.AgentMessageFailed, map[string]any{
		"error": errMsg,
	}); updateErr != nil {
		slog.Error("agent message worker: update failed", "msg_id", msgID, "error", updateErr)
	}
}
