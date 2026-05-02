package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"strait/internal/domain"
)

// ---------------------------------------------------------------------------
// Mock: evalStore
// ---------------------------------------------------------------------------.

type mockEvalStore struct {
	goldenSet      *domain.GoldenSet
	createdEvalRun *domain.EvalRun
	runs           map[string]*domain.JobRun
}

func (m *mockEvalStore) GetGoldenSet(_ context.Context, agentID, name string) (*domain.GoldenSet, error) {
	if m.goldenSet != nil && m.goldenSet.AgentID == agentID && m.goldenSet.Name == name {
		return m.goldenSet, nil
	}
	return nil, nil
}

func (m *mockEvalStore) CreateEvalRun(_ context.Context, run *domain.EvalRun) error {
	run.ID = "eval-run-1"
	run.CreatedAt = time.Now()
	m.createdEvalRun = run
	return nil
}

func (m *mockEvalStore) GetRun(_ context.Context, id string) (*domain.JobRun, error) {
	if r, ok := m.runs[id]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("run not found: %s", id)
}

// ---------------------------------------------------------------------------
// Mock: Service (sequencing mock that cycles through configured runs)
// ---------------------------------------------------------------------------.

type sequencingMockService struct {
	runIDs  []string
	callIdx int
	runErr  error
}

func (s *sequencingMockService) CreateAgent(context.Context, CreateAgentRequest) (*domain.Agent, error) {
	return nil, nil
}
func (s *sequencingMockService) GetAgent(context.Context, string, string) (*domain.Agent, error) {
	return nil, nil
}
func (s *sequencingMockService) ListAgents(context.Context, string, int, *time.Time) ([]domain.Agent, error) {
	return nil, nil
}
func (s *sequencingMockService) UpdateAgent(context.Context, UpdateAgentRequest) (*domain.Agent, error) {
	return nil, nil
}
func (s *sequencingMockService) DeleteAgent(context.Context, string, string) error { return nil }
func (s *sequencingMockService) DeployAgent(context.Context, string, string, string) (*domain.AgentDeployment, error) {
	return nil, nil
}
func (s *sequencingMockService) DeployAgentToEnv(context.Context, string, string, string, string) (*domain.AgentDeployment, error) {
	return nil, nil
}
func (s *sequencingMockService) RunAgent(_ context.Context, _ RunAgentRequest) (*domain.JobRun, error) {
	if s.runErr != nil {
		return nil, s.runErr
	}
	id := "run-fallback"
	if s.callIdx < len(s.runIDs) {
		id = s.runIDs[s.callIdx]
	}
	s.callIdx++
	return &domain.JobRun{ID: id, Status: domain.StatusQueued}, nil
}
func (s *sequencingMockService) PrepareDirectRun(context.Context, RunAgentRequest) (*DirectRunResult, error) {
	return nil, nil
}
func (s *sequencingMockService) ListAgentRuns(context.Context, string, string, int, int) ([]domain.JobRun, error) {
	return nil, nil
}
func (s *sequencingMockService) ReplayAgentRun(context.Context, ReplayAgentRunRequest) (*domain.JobRun, error) {
	return nil, nil
}
func (s *sequencingMockService) KillAgent(context.Context, string, string, string) (int, error) {
	return 0, nil
}
func (s *sequencingMockService) EnableAgent(context.Context, string, string, string) error {
	return nil
}
func (s *sequencingMockService) Close() {}

// ---------------------------------------------------------------------------
// matchesExpected tests
// ---------------------------------------------------------------------------.

func TestMatchesExpected_ContainsPass(t *testing.T) {
	t.Parallel()
	expected := json.RawMessage(`{"contains":["hello","world"]}`)
	if !matchesExpected("hello beautiful world", expected) {
		t.Fatal("expected pass")
	}
}

func TestMatchesExpected_ContainsFail(t *testing.T) {
	t.Parallel()
	expected := json.RawMessage(`{"contains":["hello","missing"]}`)
	if matchesExpected("hello world", expected) {
		t.Fatal("expected fail")
	}
}

func TestMatchesExpected_NotContainsPass(t *testing.T) {
	t.Parallel()
	expected := json.RawMessage(`{"not_contains":["forbidden","blocked"]}`)
	if !matchesExpected("hello world", expected) {
		t.Fatal("expected pass")
	}
}

func TestMatchesExpected_NotContainsFail(t *testing.T) {
	t.Parallel()
	expected := json.RawMessage(`{"not_contains":["world"]}`)
	if matchesExpected("hello world", expected) {
		t.Fatal("expected fail")
	}
}

func TestMatchesExpected_EqualsPass(t *testing.T) {
	t.Parallel()
	expected := json.RawMessage(`{"equals":"Exact Match"}`)
	if !matchesExpected("exact match", expected) {
		t.Fatal("expected case-insensitive equals to pass")
	}
}

func TestMatchesExpected_EqualsFail(t *testing.T) {
	t.Parallel()
	expected := json.RawMessage(`{"equals":"exact match"}`)
	if matchesExpected("not exact match", expected) {
		t.Fatal("expected fail")
	}
}

func TestMatchesExpected_EqualsCaseInsensitive(t *testing.T) {
	t.Parallel()
	expected := json.RawMessage(`{"equals":"Hello"}`)
	if !matchesExpected("hello", expected) {
		t.Fatal("expected case-insensitive equals to match")
	}
	if !matchesExpected("HELLO", expected) {
		t.Fatal("expected case-insensitive equals to match uppercase")
	}
	if matchesExpected("helloX", expected) {
		t.Fatal("expected non-equal string to fail")
	}
}

func TestMatchesExpected_EmptyExpected(t *testing.T) {
	t.Parallel()
	expected := json.RawMessage(`{}`)
	if !matchesExpected("anything goes", expected) {
		t.Fatal("expected pass for empty criteria")
	}
}

// ---------------------------------------------------------------------------
// RunEval tests
// ---------------------------------------------------------------------------.

func TestRunEval_AllPass(t *testing.T) {
	t.Parallel()

	cases := `[
		{"id":"c1","input":{},"expected_output":{"contains":["hello"]}},
		{"id":"c2","input":{},"expected_output":{"contains":["world"]}},
		{"id":"c3","input":{},"expected_output":{"contains":["foo"]}}
	]`

	store := &mockEvalStore{
		goldenSet: &domain.GoldenSet{
			AgentID: "agent-1", Name: "default",
			Cases: json.RawMessage(cases),
		},
		runs: map[string]*domain.JobRun{
			"run-1": {ID: "run-1", Status: domain.StatusCompleted, Result: json.RawMessage(`"hello world foo"`)},
			"run-2": {ID: "run-2", Status: domain.StatusCompleted, Result: json.RawMessage(`"world bar"`)},
			"run-3": {ID: "run-3", Status: domain.StatusCompleted, Result: json.RawMessage(`"foo baz"`)},
		},
	}
	svc := &sequencingMockService{runIDs: []string{"run-1", "run-2", "run-3"}}
	runner := NewEvalRunner(store, svc)

	result, err := runner.RunEval(context.Background(), RunEvalRequest{
		ProjectID:     "proj-1",
		AgentID:       "agent-1",
		GoldenSetName: "default",
		Actor:         "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.QualityScore != 100 {
		t.Fatalf("expected quality 100, got %.2f", result.QualityScore)
	}
	if result.PassedCases != 3 {
		t.Fatalf("expected 3 passed, got %d", result.PassedCases)
	}
	if result.FailedCases != 0 {
		t.Fatalf("expected 0 failed, got %d", result.FailedCases)
	}
}

func TestRunEval_PartialFail(t *testing.T) {
	t.Parallel()

	cases := `[
		{"id":"c1","input":{},"expected_output":{"contains":["hello"]}},
		{"id":"c2","input":{},"expected_output":{"contains":["MISSING"]}},
		{"id":"c3","input":{},"expected_output":{"contains":["foo"]}}
	]`

	store := &mockEvalStore{
		goldenSet: &domain.GoldenSet{
			AgentID: "agent-1", Name: "default",
			Cases: json.RawMessage(cases),
		},
		runs: map[string]*domain.JobRun{
			"run-1": {ID: "run-1", Status: domain.StatusCompleted, Result: json.RawMessage(`"hello world"`)},
			"run-2": {ID: "run-2", Status: domain.StatusCompleted, Result: json.RawMessage(`"no match here"`)},
			"run-3": {ID: "run-3", Status: domain.StatusCompleted, Result: json.RawMessage(`"foo bar"`)},
		},
	}
	svc := &sequencingMockService{runIDs: []string{"run-1", "run-2", "run-3"}}
	runner := NewEvalRunner(store, svc)

	result, err := runner.RunEval(context.Background(), RunEvalRequest{
		ProjectID: "proj-1", AgentID: "agent-1", Actor: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PassedCases != 2 {
		t.Fatalf("expected 2 passed, got %d", result.PassedCases)
	}
	if result.FailedCases != 1 {
		t.Fatalf("expected 1 failed, got %d", result.FailedCases)
	}
	// quality = 2/3 * 100 = 66.67
	if result.QualityScore < 66 || result.QualityScore > 67 {
		t.Fatalf("expected quality ~66.67, got %.2f", result.QualityScore)
	}
}

func TestRunEval_NoGoldenSet(t *testing.T) {
	t.Parallel()

	store := &mockEvalStore{} // no golden set
	svc := &sequencingMockService{}
	runner := NewEvalRunner(store, svc)

	_, err := runner.RunEval(context.Background(), RunEvalRequest{
		ProjectID: "proj-1", AgentID: "agent-1", Actor: "test",
	})
	if err == nil {
		t.Fatal("expected error for missing golden set")
	}
}

func TestRunEval_EmptyCases(t *testing.T) {
	t.Parallel()

	store := &mockEvalStore{
		goldenSet: &domain.GoldenSet{
			AgentID: "agent-1", Name: "default",
			Cases: json.RawMessage(`[]`),
		},
	}
	svc := &sequencingMockService{}
	runner := NewEvalRunner(store, svc)

	_, err := runner.RunEval(context.Background(), RunEvalRequest{
		ProjectID: "proj-1", AgentID: "agent-1", Actor: "test",
	})
	if err == nil {
		t.Fatal("expected error for empty cases")
	}
}

func TestRunEval_RunAgentError(t *testing.T) {
	t.Parallel()

	cases := `[{"id":"c1","input":{},"expected_output":{"contains":["hello"]}}]`

	store := &mockEvalStore{
		goldenSet: &domain.GoldenSet{
			AgentID: "agent-1", Name: "default",
			Cases: json.RawMessage(cases),
		},
	}
	svc := &sequencingMockService{runErr: fmt.Errorf("agent disabled")}
	runner := NewEvalRunner(store, svc)

	result, err := runner.RunEval(context.Background(), RunEvalRequest{
		ProjectID: "proj-1", AgentID: "agent-1", Actor: "test",
	})
	if err != nil {
		t.Fatalf("RunEval should not fail, got: %v", err)
	}
	if result.PassedCases != 0 {
		t.Fatalf("expected 0 passed, got %d", result.PassedCases)
	}
	if result.FailedCases != 1 {
		t.Fatalf("expected 1 failed, got %d", result.FailedCases)
	}
}

func TestRunEval_MaxCasesExceeded(t *testing.T) {
	t.Parallel()

	// Build a golden set with 101 cases programmatically.
	type goldenCase struct {
		ID             string          `json:"id"`
		Input          json.RawMessage `json:"input"`
		ExpectedOutput json.RawMessage `json:"expected_output"`
	}
	cases := make([]goldenCase, 101)
	for i := range cases {
		cases[i] = goldenCase{
			ID:             fmt.Sprintf("c%d", i),
			Input:          json.RawMessage(`{}`),
			ExpectedOutput: json.RawMessage(`{"contains":["ok"]}`),
		}
	}
	casesJSON, _ := json.Marshal(cases)

	store := &mockEvalStore{
		goldenSet: &domain.GoldenSet{
			AgentID: "agent-1", Name: "default",
			Cases: casesJSON,
		},
	}
	svc := &sequencingMockService{}
	runner := NewEvalRunner(store, svc)

	_, err := runner.RunEval(context.Background(), RunEvalRequest{
		ProjectID: "proj-1", AgentID: "agent-1", Actor: "test",
	})
	if err == nil {
		t.Fatal("expected error for >100 golden set cases")
	}
}
