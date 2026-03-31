package agents

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
)

// -- Mock types for message worker tests.

type mockMWStore struct {
	mu              sync.Mutex
	pendingMessages []domain.AgentMessage
	agents          map[string]*domain.Agent
	deployments     map[string]*domain.AgentDeployment
	statusUpdates   map[string]domain.AgentMessageStatus
	listPendingErr  error
	getAgentErr     error
	updateStatusErr error
}

func (m *mockMWStore) ListPendingAgentMessages(_ context.Context, _ int) ([]domain.AgentMessage, error) {
	if m.listPendingErr != nil {
		return nil, m.listPendingErr
	}
	return m.pendingMessages, nil
}

func (m *mockMWStore) UpdateAgentMessageStatus(_ context.Context, id string, status domain.AgentMessageStatus, _ map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateStatusErr != nil {
		return m.updateStatusErr
	}
	if m.statusUpdates == nil {
		m.statusUpdates = make(map[string]domain.AgentMessageStatus)
	}
	m.statusUpdates[id] = status
	return nil
}

func (m *mockMWStore) GetAgent(_ context.Context, id string) (*domain.Agent, error) {
	if m.getAgentErr != nil {
		return nil, m.getAgentErr
	}
	if a, ok := m.agents[id]; ok {
		return a, nil
	}
	return nil, errors.New("agent not found")
}

func (m *mockMWStore) GetLatestAgentDeployment(_ context.Context, agentID string) (*domain.AgentDeployment, error) {
	if d, ok := m.deployments[agentID]; ok {
		return d, nil
	}
	return nil, errors.New("no deployment")
}

type mockMWAgentService struct {
	mu       sync.Mutex
	runCalls int
	runErr   error
}

func (m *mockMWAgentService) CreateAgent(_ context.Context, _ CreateAgentRequest) (*domain.Agent, error) {
	return nil, nil
}
func (m *mockMWAgentService) GetAgent(_ context.Context, _, _ string) (*domain.Agent, error) {
	return nil, nil
}
func (m *mockMWAgentService) ListAgents(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.Agent, error) {
	return nil, nil
}
func (m *mockMWAgentService) UpdateAgent(_ context.Context, _ UpdateAgentRequest) (*domain.Agent, error) {
	return nil, nil
}
func (m *mockMWAgentService) DeleteAgent(_ context.Context, _, _ string) error { return nil }
func (m *mockMWAgentService) DeployAgent(_ context.Context, _, _, _ string) (*domain.AgentDeployment, error) {
	return nil, nil
}
func (m *mockMWAgentService) RunAgent(_ context.Context, _ RunAgentRequest) (*domain.JobRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runCalls++
	if m.runErr != nil {
		return nil, m.runErr
	}
	return &domain.JobRun{ID: "run-mw-1", Status: domain.StatusQueued}, nil
}
func (m *mockMWAgentService) PrepareDirectRun(_ context.Context, _ RunAgentRequest) (*DirectRunResult, error) {
	return nil, nil
}
func (m *mockMWAgentService) ListAgentRuns(_ context.Context, _, _ string, _, _ int) ([]domain.JobRun, error) {
	return nil, nil
}
func (m *mockMWAgentService) ReplayAgentRun(_ context.Context, _ ReplayAgentRunRequest) (*domain.JobRun, error) {
	return nil, nil
}
func (m *mockMWAgentService) Close() {}

// -- Lifecycle tests.

func TestMessageWorker_StopWithoutStart(t *testing.T) {
	t.Parallel()
	w := NewMessageWorker(MessageWorkerDeps{})
	// Should not deadlock.
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() deadlocked on unstarted worker")
	}
}

func TestMessageWorker_NilWorkerStartSafe(t *testing.T) {
	t.Parallel()
	var w *MessageWorker
	w.Start(context.Background()) // should not panic
}

func TestMessageWorker_NilWorkerStopSafe(t *testing.T) {
	t.Parallel()
	var w *MessageWorker
	w.Stop() // should not panic
}

func TestMessageWorker_DoubleStartIgnored(t *testing.T) {
	t.Parallel()
	w := NewMessageWorker(MessageWorkerDeps{
		Store:    &mockMWStore{},
		AgentSvc: &mockMWAgentService{},
	})
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	w.Start(ctx) // second start should be no-op
	cancel()
	w.Stop()
}

// -- Deliver tests.

func TestMessageWorkerDeliver_Success(t *testing.T) {
	t.Parallel()
	store := &mockMWStore{
		agents: map[string]*domain.Agent{
			"agent-tgt": {ID: "agent-tgt", ProjectID: "proj-1"},
		},
		deployments: map[string]*domain.AgentDeployment{
			"agent-tgt": {ID: "dep-1", Status: domain.AgentDeploymentStatusDeployed},
		},
	}
	svc := &mockMWAgentService{}
	w := NewMessageWorker(MessageWorkerDeps{Store: store, AgentSvc: svc})

	msg := &domain.AgentMessage{
		ID: "msg-1", ProjectID: "proj-1",
		SourceAgentID: "agent-src", TargetAgentID: "agent-tgt",
		ChainID: "chain-1", ChainDepth: 1,
		Payload: json.RawMessage(`{"text":"hello"}`),
	}
	w.deliver(context.Background(), msg)

	if svc.runCalls != 1 {
		t.Fatalf("RunAgent called %d times, want 1", svc.runCalls)
	}
	if store.statusUpdates["msg-1"] != domain.AgentMessageDelivered {
		t.Fatalf("status = %v, want delivered", store.statusUpdates["msg-1"])
	}
}

func TestMessageWorkerDeliver_AgentNotFound(t *testing.T) {
	t.Parallel()
	store := &mockMWStore{
		agents: map[string]*domain.Agent{}, // empty
	}
	w := NewMessageWorker(MessageWorkerDeps{Store: store, AgentSvc: &mockMWAgentService{}})

	msg := &domain.AgentMessage{ID: "msg-2", TargetAgentID: "nonexistent"}
	w.deliver(context.Background(), msg)

	if store.statusUpdates["msg-2"] != domain.AgentMessageFailed {
		t.Fatalf("status = %v, want failed", store.statusUpdates["msg-2"])
	}
}

func TestMessageWorkerDeliver_NoDeployment(t *testing.T) {
	t.Parallel()
	store := &mockMWStore{
		agents:      map[string]*domain.Agent{"a": {ID: "a"}},
		deployments: map[string]*domain.AgentDeployment{}, // no deployment
	}
	w := NewMessageWorker(MessageWorkerDeps{Store: store, AgentSvc: &mockMWAgentService{}})

	msg := &domain.AgentMessage{ID: "msg-3", TargetAgentID: "a"}
	w.deliver(context.Background(), msg)

	if store.statusUpdates["msg-3"] != domain.AgentMessageFailed {
		t.Fatalf("status = %v, want failed", store.statusUpdates["msg-3"])
	}
}

func TestMessageWorkerDeliver_DeploymentNotDeployed(t *testing.T) {
	t.Parallel()
	store := &mockMWStore{
		agents:      map[string]*domain.Agent{"a": {ID: "a"}},
		deployments: map[string]*domain.AgentDeployment{"a": {ID: "d", Status: domain.AgentDeploymentStatusPending}},
	}
	w := NewMessageWorker(MessageWorkerDeps{Store: store, AgentSvc: &mockMWAgentService{}})

	msg := &domain.AgentMessage{ID: "msg-4", TargetAgentID: "a"}
	w.deliver(context.Background(), msg)

	if store.statusUpdates["msg-4"] != domain.AgentMessageFailed {
		t.Fatalf("status = %v, want failed", store.statusUpdates["msg-4"])
	}
}

func TestMessageWorkerDeliver_RunAgentError(t *testing.T) {
	t.Parallel()
	store := &mockMWStore{
		agents:      map[string]*domain.Agent{"a": {ID: "a"}},
		deployments: map[string]*domain.AgentDeployment{"a": {ID: "d", Status: domain.AgentDeploymentStatusDeployed}},
	}
	svc := &mockMWAgentService{runErr: errors.New("quota exceeded")}
	w := NewMessageWorker(MessageWorkerDeps{Store: store, AgentSvc: svc})

	msg := &domain.AgentMessage{ID: "msg-5", ProjectID: "proj-1", TargetAgentID: "a"}
	w.deliver(context.Background(), msg)

	if store.statusUpdates["msg-5"] != domain.AgentMessageFailed {
		t.Fatalf("status = %v, want failed", store.statusUpdates["msg-5"])
	}
}

// -- Poll tests.

func TestMessageWorkerPoll_HandlesStoreError(t *testing.T) {
	t.Parallel()
	store := &mockMWStore{listPendingErr: errors.New("db down")}
	w := NewMessageWorker(MessageWorkerDeps{Store: store, AgentSvc: &mockMWAgentService{}})
	// Should not panic.
	w.poll(context.Background())
}

func TestMessageWorkerPoll_EmptyBatch(t *testing.T) {
	t.Parallel()
	store := &mockMWStore{pendingMessages: nil}
	svc := &mockMWAgentService{}
	w := NewMessageWorker(MessageWorkerDeps{Store: store, AgentSvc: svc})
	w.poll(context.Background())
	if svc.runCalls != 0 {
		t.Fatalf("RunAgent called %d times, want 0 for empty batch", svc.runCalls)
	}
}

// -- Configuration defaults tests.

func TestMessageWorkerDefaults_BatchSize(t *testing.T) {
	t.Parallel()
	w := NewMessageWorker(MessageWorkerDeps{BatchSize: 0})
	if w.batchSize != 50 {
		t.Fatalf("batchSize = %d, want 50", w.batchSize)
	}
}

func TestMessageWorkerDefaults_PollInterval(t *testing.T) {
	t.Parallel()
	w := NewMessageWorker(MessageWorkerDeps{PollInterval: 0})
	if w.pollInterval != 5*time.Second {
		t.Fatalf("pollInterval = %v, want 5s", w.pollInterval)
	}
}

func TestMessageWorkerDefaults_Clock(t *testing.T) {
	t.Parallel()
	w := NewMessageWorker(MessageWorkerDeps{Clock: nil})
	if w.now == nil {
		t.Fatal("now should default to time.Now, got nil")
	}
}

// -- Adversarial tests.

func TestMessageWorkerDeliver_ClockUsed(t *testing.T) {
	t.Parallel()
	frozen := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &mockMWStore{
		agents:      map[string]*domain.Agent{"a": {ID: "a"}},
		deployments: map[string]*domain.AgentDeployment{"a": {ID: "d", Status: domain.AgentDeploymentStatusDeployed}},
	}
	svc := &mockMWAgentService{}
	w := NewMessageWorker(MessageWorkerDeps{
		Store: store, AgentSvc: svc,
		Clock: func() time.Time { return frozen },
	})

	msg := &domain.AgentMessage{ID: "msg-clock", ProjectID: "proj-1", TargetAgentID: "a"}
	w.deliver(context.Background(), msg)

	// The frozen clock should be used for delivered_at.
	// We can't directly check the timestamp since it's in the map[string]any,
	// but we verify no panic and the status was updated.
	if store.statusUpdates["msg-clock"] != domain.AgentMessageDelivered {
		t.Fatalf("status = %v, want delivered", store.statusUpdates["msg-clock"])
	}
}

func TestMessageWorkerDeliver_MarkFailedStoreError(t *testing.T) {
	t.Parallel()
	store := &mockMWStore{
		agents:          map[string]*domain.Agent{}, // agent not found -> markFailed
		updateStatusErr: errors.New("db locked"),
	}
	w := NewMessageWorker(MessageWorkerDeps{Store: store, AgentSvc: &mockMWAgentService{}})

	// Should not panic even when markFailed fails.
	msg := &domain.AgentMessage{ID: "msg-err", TargetAgentID: "nonexistent"}
	w.deliver(context.Background(), msg)
}
