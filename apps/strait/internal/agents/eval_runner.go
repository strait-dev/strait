package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"strait/internal/domain"
)

// EvalStore defines the store methods needed by the eval runner.
type EvalStore interface {
	GetGoldenSet(ctx context.Context, agentID, name string) (*domain.GoldenSet, error)
	CreateEvalRun(ctx context.Context, run *domain.EvalRun) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
}

// EvalRunner executes golden-set evaluations against agents.
type EvalRunner struct {
	store   EvalStore
	service Service
}

// NewEvalRunner creates a new eval runner.
func NewEvalRunner(store EvalStore, service Service) *EvalRunner {
	return &EvalRunner{store: store, service: service}
}

// RunEvalRequest contains the parameters for running an evaluation.
type RunEvalRequest struct {
	ProjectID     string
	AgentID       string
	GoldenSetName string // default: "default"
	Model         string // optional: override model for comparison
	TimeoutSecs   int    // per-case timeout, default 60
	Actor         string
}

// RunEvalResult contains the outcome of an evaluation run.
type RunEvalResult struct {
	EvalRunID    string                  `json:"eval_run_id"`
	QualityScore float64                 `json:"quality_score"`
	TotalCases   int                     `json:"total_cases"`
	PassedCases  int                     `json:"passed_cases"`
	FailedCases  int                     `json:"failed_cases"`
	Results      []domain.EvalCaseResult `json:"results"`
	DurationMs   int                     `json:"duration_ms"`
}

// RunEval executes a golden set against an agent and returns quality metrics.
func (r *EvalRunner) RunEval(ctx context.Context, req RunEvalRequest) (*RunEvalResult, error) {
	if req.GoldenSetName == "" {
		req.GoldenSetName = "default"
	}
	if req.TimeoutSecs <= 0 {
		req.TimeoutSecs = 60
	}

	// Load golden set.
	gs, err := r.store.GetGoldenSet(ctx, req.AgentID, req.GoldenSetName)
	if err != nil {
		return nil, fmt.Errorf("load golden set: %w", err)
	}
	if gs == nil {
		return nil, fmt.Errorf("golden set %q not found for agent %s", req.GoldenSetName, req.AgentID)
	}

	// Parse cases.
	var cases []domain.GoldenCase
	if err := json.Unmarshal(gs.Cases, &cases); err != nil {
		return nil, fmt.Errorf("parse golden set cases: %w", err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("golden set %q has no cases", req.GoldenSetName)
	}

	start := time.Now()
	results := make([]domain.EvalCaseResult, 0, len(cases))
	passed := 0

	for _, tc := range cases {
		caseStart := time.Now()
		caseCtx, cancel := context.WithTimeout(ctx, time.Duration(req.TimeoutSecs)*time.Second)

		result := r.runCase(caseCtx, req, tc)
		result.LatencyMs = int(time.Since(caseStart).Milliseconds())
		cancel()

		if result.Passed {
			passed++
		}
		results = append(results, result)
	}

	totalDuration := int(time.Since(start).Milliseconds())
	failed := len(cases) - passed
	qualityScore := float64(0)
	if len(cases) > 0 {
		qualityScore = float64(passed) / float64(len(cases)) * 100
	}

	// Persist as EvalRun.
	resultsJSON, _ := json.Marshal(results)
	evalRun := &domain.EvalRun{
		AgentID:     req.AgentID,
		ProjectID:   req.ProjectID,
		SuiteName:   req.GoldenSetName,
		ResultsJSON: resultsJSON,
		TotalCases:  len(cases),
		PassedCases: passed,
		FailedCases: failed,
		DurationMs:  totalDuration,
		Status:      "completed",
	}
	if createErr := r.store.CreateEvalRun(ctx, evalRun); createErr != nil {
		return nil, fmt.Errorf("store eval run: %w", createErr)
	}

	return &RunEvalResult{
		EvalRunID:    evalRun.ID,
		QualityScore: qualityScore,
		TotalCases:   len(cases),
		PassedCases:  passed,
		FailedCases:  failed,
		Results:      results,
		DurationMs:   totalDuration,
	}, nil
}

// runCase executes a single golden case and checks the result.
func (r *EvalRunner) runCase(ctx context.Context, req RunEvalRequest, tc domain.GoldenCase) domain.EvalCaseResult {
	result := domain.EvalCaseResult{CaseID: tc.ID}

	// Trigger agent run with the case input as payload.
	run, err := r.service.RunAgent(ctx, RunAgentRequest{
		ProjectID: req.ProjectID,
		AgentID:   req.AgentID,
		Payload:   tc.Input,
		Actor:     "eval:" + req.Actor,
	})
	if err != nil {
		result.Reason = fmt.Sprintf("run failed: %v", err)
		return result
	}

	// Poll for completion.
	var finalRun *domain.JobRun
	for {
		select {
		case <-ctx.Done():
			result.Reason = "timeout waiting for run completion"
			return result
		default:
		}

		r2, rErr := r.store.GetRun(ctx, run.ID)
		if rErr != nil {
			result.Reason = fmt.Sprintf("poll error: %v", rErr)
			return result
		}
		if r2.Status.IsTerminal() || r2.Status == domain.StatusDeadLetter {
			finalRun = r2
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if finalRun.Status != domain.StatusCompleted {
		result.Reason = fmt.Sprintf("run ended with status %s: %s", finalRun.Status, finalRun.Error)
		return result
	}

	// Compare output against expected.
	actualOutput := string(finalRun.Result)
	result.Actual = actualOutput
	result.Passed = matchesExpected(actualOutput, tc.ExpectedOutput)
	if !result.Passed {
		result.Reason = "output did not match expected criteria"
	}
	return result
}

// matchesExpected checks if actual output satisfies the expected output criteria.
// Expected output can contain:
//   - {"contains": ["keyword1", "keyword2"]} — output must include all
//   - {"not_contains": ["bad"]} — output must not include any
//   - {"equals": "exact match"} — exact string match
func matchesExpected(actual string, expected json.RawMessage) bool {
	if len(expected) == 0 {
		return true // no expectations = pass
	}

	var criteria struct {
		Contains    []string `json:"contains"`
		NotContains []string `json:"not_contains"`
		Equals      string   `json:"equals"`
	}
	if err := json.Unmarshal(expected, &criteria); err != nil {
		return false
	}

	lowerActual := strings.ToLower(actual)

	if criteria.Equals != "" {
		return actual == criteria.Equals
	}

	for _, kw := range criteria.Contains {
		if !strings.Contains(lowerActual, strings.ToLower(kw)) {
			return false
		}
	}
	for _, kw := range criteria.NotContains {
		if strings.Contains(lowerActual, strings.ToLower(kw)) {
			return false
		}
	}
	return true
}
