package authoring

import (
	"context"
	"sync"
)

// TestRunRecord captures all operations on a test RunContext.
type TestRunRecord struct {
	mu              sync.Mutex
	Checkpoints     []map[string]any
	Logs            []map[string]any
	UsageReports    []map[string]any
	ToolCalls       []map[string]any
	Outputs         []map[string]any
	ProgressUpdates []map[string]any
	StateStore      map[string]any
	StreamChunks    []map[string]any
	Heartbeats      int
	Spawns          []map[string]any
	Events          []map[string]any
	Annotations     []map[string]string
	Continuations   []map[string]any
	Completed       bool
	Failed          bool
	FailError       string
	Result          map[string]any
}

type mockRunContextClient struct {
	record *TestRunRecord
}

func (m *mockRunContextClient) CheckpointRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	if b, ok := body.(map[string]any); ok {
		if state, ok := b["state"].(map[string]any); ok {
			m.record.Checkpoints = append(m.record.Checkpoints, state)
		}
	}
	return nil, nil
}

func (m *mockRunContextClient) HeartbeatRun(_ context.Context, _ string) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	m.record.Heartbeats++
	return nil, nil
}

func (m *mockRunContextClient) ProgressRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	if b, ok := body.(map[string]any); ok {
		m.record.ProgressUpdates = append(m.record.ProgressUpdates, b)
	}
	return nil, nil
}

func (m *mockRunContextClient) LogRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	if b, ok := body.(map[string]any); ok {
		m.record.Logs = append(m.record.Logs, b)
	}
	return nil, nil
}

func (m *mockRunContextClient) UsageRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	if b, ok := body.(map[string]any); ok {
		m.record.UsageReports = append(m.record.UsageReports, b)
	}
	return nil, nil
}

func (m *mockRunContextClient) ToolCallRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	if b, ok := body.(map[string]any); ok {
		m.record.ToolCalls = append(m.record.ToolCalls, b)
	}
	return nil, nil
}

func (m *mockRunContextClient) OutputRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	if b, ok := body.(map[string]any); ok {
		m.record.Outputs = append(m.record.Outputs, b)
	}
	return nil, nil
}

func (m *mockRunContextClient) WaitForEventRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	if b, ok := body.(map[string]any); ok {
		m.record.Events = append(m.record.Events, b)
	}
	return map[string]any{"received": true}, nil
}

func (m *mockRunContextClient) SpawnRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	if b, ok := body.(map[string]any); ok {
		m.record.Spawns = append(m.record.Spawns, b)
	}
	return map[string]any{"run_id": "spawned-123"}, nil
}

func (m *mockRunContextClient) ContinueRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	if b, ok := body.(map[string]any); ok {
		m.record.Continuations = append(m.record.Continuations, b)
	}
	return map[string]any{"continued": true}, nil
}

func (m *mockRunContextClient) AnnotateRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	if b, ok := body.(map[string]any); ok {
		if annotations, ok := b["annotations"].(map[string]string); ok {
			m.record.Annotations = append(m.record.Annotations, annotations)
		}
	}
	return nil, nil
}

func (m *mockRunContextClient) CompleteRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	m.record.Completed = true
	if b, ok := body.(map[string]any); ok {
		if result, ok := b["result"].(map[string]any); ok {
			m.record.Result = result
		}
	}
	return nil, nil
}

func (m *mockRunContextClient) FailRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	m.record.Failed = true
	if b, ok := body.(map[string]any); ok {
		if errMsg, ok := b["error"].(string); ok {
			m.record.FailError = errMsg
		}
	}
	return nil, nil
}

func (m *mockRunContextClient) SetState(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	if b, ok := body.(map[string]any); ok {
		key, _ := b["key"].(string)
		m.record.StateStore[key] = b["value"]
	}
	return nil, nil
}

func (m *mockRunContextClient) ListState(_ context.Context, _ string) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	var items []any
	for k, v := range m.record.StateStore {
		items = append(items, map[string]any{"key": k, "value": v})
	}
	return map[string]any{"data": items}, nil
}

func (m *mockRunContextClient) GetState(_ context.Context, _ string, key string) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	val := m.record.StateStore[key]
	return map[string]any{"key": key, "value": val}, nil
}

func (m *mockRunContextClient) DeleteState(_ context.Context, _ string, key string) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	delete(m.record.StateStore, key)
	return nil, nil
}

func (m *mockRunContextClient) StreamRun(_ context.Context, _ string, body any) (map[string]any, error) {
	m.record.mu.Lock()
	defer m.record.mu.Unlock()
	if b, ok := body.(map[string]any); ok {
		m.record.StreamChunks = append(m.record.StreamChunks, b)
	}
	return nil, nil
}

// CreateTestContext creates an in-memory RunContext and TestRunRecord for testing.
func CreateTestContext(runID string, opts ...RunContextOption) (RunContext, *TestRunRecord) {
	record := &TestRunRecord{
		StateStore: make(map[string]any),
	}
	mock := &mockRunContextClient{record: record}
	ctx := CreateRunContext(mock, runID, opts...)
	return ctx, record
}
