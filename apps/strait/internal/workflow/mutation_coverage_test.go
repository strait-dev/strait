package workflow

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMutationCoverage_ConditionResultHelpers(t *testing.T) {
	t.Parallel()

	statuses := map[string]domain.StepRunStatus{
		"completed": domain.StepCompleted,
		"failed":    domain.StepFailed,
	}

	got, err := evaluateStepStatusConditionResult(
		gjson.Parse(`{"type":"step_status","step_ref":"completed","status":"completed"}`),
		statuses,
	)
	require.NoError(t, err)
	assert.True(t, got)

	got, err = evaluateStepStatusConditionResult(
		gjson.Parse(`{"type":"step_status","step_ref":"completed","status":"failed"}`),
		statuses,
	)
	require.NoError(t, err)
	assert.False(t, got)

	_, err = evaluateStepStatusConditionResult(gjson.Parse(`{"type":"step_status","status":"completed"}`), statuses)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step_ref is required")

	_, err = evaluateStepStatusConditionResult(
		gjson.Parse(`{"type":"step_status","step_ref":"missing","status":"completed"}`),
		statuses,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `step "missing" not found`)

	_, err = evaluateStepStatusConditionResult(gjson.Parse(`{"type":"step_status","step_ref":1}`), statuses)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal step_status condition")
}

func TestMutationCoverage_StepStatusInResultBranches(t *testing.T) {
	t.Parallel()

	statuses := map[string]domain.StepRunStatus{"step": domain.StepCompleted}
	cond := gjson.Parse(`{"type":"step_status_in","step_ref":"step","statuses":["failed","completed"]}`)
	got, err := evaluateStepStatusInConditionResult(cond, statuses)
	require.NoError(t, err)
	assert.True(t, got)

	cond = gjson.Parse(`{"type":"step_status_in","step_ref":"step","statuses":["failed"]}`)
	got, err = evaluateStepStatusInConditionResult(cond, statuses)
	require.NoError(t, err)
	assert.False(t, got)

	_, err = evaluateStepStatusInConditionResult(gjson.Parse(`{"type":"step_status_in","statuses":["completed"]}`), statuses)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step_ref is required")

	_, err = evaluateStepStatusInConditionResult(
		gjson.Parse(`{"type":"step_status_in","step_ref":"missing","statuses":["completed"]}`),
		statuses,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `step "missing" not found`)

	_, err = evaluateStepStatusInConditionResult(
		gjson.Parse(`{"type":"step_status_in","step_ref":"step","statuses":["failed",1]}`),
		statuses,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal step_status_in condition")
}

func TestMutationCoverage_CompositeConditionResultBranches(t *testing.T) {
	t.Parallel()

	statuses := map[string]domain.StepRunStatus{
		"a": domain.StepCompleted,
		"b": domain.StepFailed,
	}

	got, err := evaluateNotConditionResult(
		gjson.Parse(`{"type":"not","condition":{"type":"step_status","step_ref":"a","status":"failed"}}`),
		statuses,
	)
	require.NoError(t, err)
	assert.True(t, got)

	_, err = evaluateNotConditionResult(gjson.Parse(`{"type":"not"}`), statuses)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "condition is required")

	_, err = evaluateNotConditionResult(
		gjson.Parse(`{"type":"not","condition":{"type":"step_status","step_ref":"missing","status":"completed"}}`),
		statuses,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `step "missing" not found`)

	got, err = evaluateAllOfConditionResult(
		gjson.Parse(`{"type":"all_of","conditions":[{"type":"step_status","step_ref":"a","status":"completed"},{"type":"step_status","step_ref":"b","status":"completed"}]}`),
		statuses,
	)
	require.NoError(t, err)
	assert.False(t, got)

	got, err = evaluateAnyOfConditionResult(
		gjson.Parse(`{"type":"any_of","conditions":[{"type":"step_status","step_ref":"a","status":"failed"},{"type":"step_status","step_ref":"b","status":"failed"}]}`),
		statuses,
	)
	require.NoError(t, err)
	assert.True(t, got)

	_, err = compositeConditionsResult(gjson.Parse(`{"type":"all_of","conditions":{}}`), "all_of")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal all_of condition")
}

func TestMutationCoverage_BinaryConditionResultBranches(t *testing.T) {
	t.Parallel()

	statuses := map[string]domain.StepRunStatus{"step": domain.StepCompleted}

	tests := []struct {
		name     string
		condType string
		cond     string
		want     bool
	}{
		{name: "eq false", condType: "eq", cond: `{"type":"eq","left":1,"right":2}`},
		{name: "ne true", condType: "ne", cond: `{"type":"ne","left":1,"right":2}`, want: true},
		{name: "ne false", condType: "ne", cond: `{"type":"ne","left":"same","right":"same"}`},
		{name: "gt", condType: "gt", cond: `{"type":"gt","left":3,"right":2}`, want: true},
		{name: "gte equal", condType: "gte", cond: `{"type":"gte","left":2,"right":2}`, want: true},
		{name: "lt", condType: "lt", cond: `{"type":"lt","left":1,"right":2}`, want: true},
		{name: "lte equal", condType: "lte", cond: `{"type":"lte","left":2,"right":2}`, want: true},
		{name: "contains false", condType: "contains", cond: `{"type":"contains","left":"abc","right":"z"}`},
		{name: "in true", condType: "in", cond: `{"type":"in","left":"b","right":["a","b"]}`, want: true},
		{name: "in false", condType: "in", cond: `{"type":"in","left":"c","right":["a","b"]}`},
		{name: "regex false", condType: "regex", cond: `{"type":"regex","left":"abc","right":"^z+$"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := evaluateBinaryConditionResult(tt.condType, gjson.Parse(tt.cond), statuses)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := evaluateBinaryConditionResult("gt", gjson.Parse(`{"type":"gt","left":"x","right":2}`), statuses)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "numeric comparison requires")

	_, err = evaluateBinaryConditionResult("in", gjson.Parse(`{"type":"in","left":"x","right":"not-array"}`), statuses)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "in requires right operand array")

	got, err := evaluateExistsConditionResult(
		gjson.Parse(`{"type":"exists","operand":{"step_ref":"step"}}`),
		statuses,
	)
	require.NoError(t, err)
	assert.True(t, got)

	got, err = evaluateExistsConditionResult(
		gjson.Parse(`{"type":"exists","operand":{"step_ref":"missing"}}`),
		statuses,
	)
	require.NoError(t, err)
	assert.False(t, got)
}

func TestMutationCoverage_FastStringConditionBranches(t *testing.T) {
	t.Parallel()

	statuses := map[string]domain.StepRunStatus{"step": domain.StepCompleted}
	cond := gjson.Parse(`{"left":{"step_ref":"step"},"right":"completed"}`)
	got, handled, err := evaluateFastStringBinaryConditionResult("eq", cond, statuses)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.True(t, got)

	got, handled, err = evaluateFastStringBinaryConditionResult("ne", cond, statuses)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.False(t, got)

	got, handled, err = evaluateFastStringBinaryConditionResult(
		"contains",
		gjson.Parse(`{"left":"workflow-step","right":"step"}`),
		statuses,
	)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.True(t, got)

	got, handled, err = evaluateFastStringBinaryConditionResult(
		"regex",
		gjson.Parse(`{"left":"job-123","right":"^job-[0-9]+$"}`),
		statuses,
	)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.True(t, got)

	_, handled, err = evaluateFastStringBinaryConditionResult(
		"regex",
		gjson.Parse(`{"left":"job","right":"["}`),
		statuses,
	)
	require.Error(t, err)
	assert.True(t, handled)

	_, handled, err = evaluateFastStringBinaryConditionResult(
		"regex",
		gjson.Parse(`{"left":"job","right":"`+strings.Repeat("a", maxRegexPatternLen+1)+`"}`),
		statuses,
	)
	require.Error(t, err)
	assert.True(t, handled)

	_, handled, err = evaluateFastStringBinaryConditionResult(
		"regex",
		gjson.Parse(`{"left":"`+strings.Repeat("a", maxRegexInputLen+1)+`","right":"a+"}`),
		statuses,
	)
	require.Error(t, err)
	assert.True(t, handled)

	got, handled, err = evaluateFastStringBinaryConditionResult(
		"gt",
		gjson.Parse(`{"left":"a","right":"b"}`),
		statuses,
	)
	require.NoError(t, err)
	assert.False(t, got)
	assert.False(t, handled)

	_, handled, err = evaluateFastStringBinaryConditionResult("eq", gjson.Parse(`{"right":"b"}`), statuses)
	require.NoError(t, err)
	assert.False(t, handled)

	_, handled, err = evaluateFastStringBinaryConditionResult("eq", gjson.Parse(`{"left":"a"}`), statuses)
	require.NoError(t, err)
	assert.False(t, handled)
}

func TestMutationCoverage_ConditionOperandStringBranches(t *testing.T) {
	t.Parallel()

	statuses := map[string]domain.StepRunStatus{"step": domain.StepCompleted}

	got, ok := conditionOperandString(gjson.Parse(`{"step_ref":"step"}`), statuses)
	require.True(t, ok)
	assert.Equal(t, "completed", got)

	got, ok = conditionOperandString(gjson.Parse(`{"step_ref":"missing"}`), statuses)
	require.True(t, ok)
	assert.Empty(t, got)

	got, ok = conditionOperandString(gjson.Parse(`{"value":true}`), statuses)
	require.True(t, ok)
	assert.Equal(t, "true", got)

	got, ok = conditionOperandString(gjson.Parse(`{"value":null}`), statuses)
	require.True(t, ok)
	assert.Empty(t, got)

	_, ok = conditionOperandString(gjson.Parse(`[1,2]`), statuses)
	assert.False(t, ok)

	_, ok = conditionOperandString(gjson.Parse(`{"value":{"nested":true}}`), statuses)
	assert.False(t, ok)
}

func TestMutationCoverage_CachedConditionRegexResetsWhenFull(t *testing.T) {
	conditionRegexCache.mu.Lock()
	conditionRegexCache.compiled = make(map[string]*regexp.Regexp, maxConditionRegexCacheEntries)
	for i := range maxConditionRegexCacheEntries {
		conditionRegexCache.compiled["pattern-"+strconv.Itoa(i)] = regexp.MustCompile("a")
	}
	conditionRegexCache.mu.Unlock()

	re, err := cachedConditionRegex("^fresh$")
	require.NoError(t, err)
	assert.True(t, re.MatchString("fresh"))

	conditionRegexCache.mu.RLock()
	defer conditionRegexCache.mu.RUnlock()
	assert.NotContains(t, conditionRegexCache.compiled, "pattern-0")
	assert.Contains(t, conditionRegexCache.compiled, "^fresh$")
}

func TestMutationCoverage_DebugViewNonAlignedFallback(t *testing.T) {
	t.Parallel()

	started := makeTime("2026-01-01T00:00:00Z")
	finished := makeTime("2026-01-01T00:00:10Z")
	stepStarted := makeTime("2026-01-01T00:00:01Z")
	stepFinished := makeTime("2026-01-01T00:00:04Z")
	wfRun := &domain.WorkflowRun{
		ID:         "wf-run-debug",
		WorkflowID: "workflow-debug",
		Status:     domain.WfStatusCompleted,
		StartedAt:  started,
		FinishedAt: finished,
		Error:      "terminal error",
		Payload:    []byte(`{"input":true}`),
	}
	steps := []domain.WorkflowStep{
		{StepRef: "source", StepType: domain.WorkflowStepTypeJob},
		{StepRef: "join", DependsOn: []string{"source"}},
	}
	stepRuns := []domain.WorkflowStepRun{
		{
			ID:         "sr-join",
			StepRef:    "join",
			Status:     domain.StepCompleted,
			JobRunID:   "job-run-join",
			Output:     []byte(`{"joined":true}`),
			StartedAt:  stepStarted,
			FinishedAt: stepFinished,
			Attempt:    2,
		},
		{
			ID:      "sr-source",
			StepRef: "source",
			Status:  domain.StepCompleted,
			Output:  []byte(`{"source":true}`),
		},
		{
			ID:      "sr-orphan",
			StepRef: "orphan",
			Status:  domain.StepFailed,
			Error:   "orphan failed",
		},
	}

	view, err := BuildDebugView(wfRun, steps, stepRuns, map[string]int64{"sr-join": 99})
	require.NoError(t, err)
	assert.EqualValues(t, 10_000, view.TotalDuration)
	assert.EqualValues(t, 99, view.TotalCost)
	require.Len(t, view.Steps, 3)
	assert.Equal(t, "job", view.Steps[0].StepType)
	assert.Equal(t, []string{"source"}, view.Steps[0].DependsOn)
	assert.EqualValues(t, 3_000, view.Steps[0].Duration)
	assert.Equal(t, "orphan failed", view.Steps[2].Error)
	require.Len(t, view.DataFlow, 1)
	assert.Equal(t, DataFlowEdge{
		FromStepRef: "source",
		ToStepRef:   "join",
		DataSize:    len(stepRuns[1].Output),
	}, view.DataFlow[0])
}

func TestMutationCoverage_DebugViewAlignedNonLinearDataFlow(t *testing.T) {
	t.Parallel()

	wfRun := &domain.WorkflowRun{ID: "wf-run-debug-linear", WorkflowID: "workflow-debug", Status: domain.WfStatusRunning}
	steps := []domain.WorkflowStep{
		{StepRef: "source"},
		{StepRef: "branch"},
		{StepRef: "join", DependsOn: []string{"source"}},
	}
	stepRuns := []domain.WorkflowStepRun{
		{ID: "sr-source", StepRef: "source", Status: domain.StepCompleted, Output: []byte(`{"source":true}`)},
		{ID: "sr-branch", StepRef: "branch", Status: domain.StepCompleted, Output: []byte(`{"branch":true}`)},
		{ID: "sr-join", StepRef: "join", Status: domain.StepPending},
	}

	view, err := BuildDebugView(wfRun, steps, stepRuns, nil)
	require.NoError(t, err)
	require.Len(t, view.DataFlow, 1)
	assert.Equal(t, DataFlowEdge{
		FromStepRef: "source",
		ToStepRef:   "join",
		DataSize:    len(stepRuns[0].Output),
	}, view.DataFlow[0])
	assert.Equal(t, "job", view.Steps[0].StepType)
}

func TestMutationCoverage_StringTemplateVarCacheResolveBranches(t *testing.T) {
	t.Parallel()

	vars := []byte(`{"name":"Ada","count":2,"flag":true,"obj":{"x":1}}`)
	var cache stringTemplateVarCache

	got, ok := cache.resolve(vars, "name")
	require.True(t, ok)
	assert.Equal(t, "Ada", got)
	assert.Equal(t, 1, cache.valueCount)

	got, ok = cache.resolve(vars, "name")
	require.True(t, ok)
	assert.Equal(t, "Ada", got)
	assert.Equal(t, 1, cache.valueCount)

	got, ok = cache.resolve(vars, "count")
	require.True(t, ok)
	assert.Equal(t, "2", got)

	got, ok = cache.resolve(vars, "missing")
	require.False(t, ok)
	assert.Empty(t, got)
	assert.Equal(t, 1, cache.missingCount)

	got, ok = cache.resolve(vars, "missing")
	require.False(t, ok)
	assert.Empty(t, got)
	assert.Equal(t, 1, cache.missingCount)

	fullMissing := stringTemplateVarCache{missingCount: 8}
	got, ok = fullMissing.resolve(vars, "still_missing")
	require.False(t, ok)
	assert.Empty(t, got)
	assert.Equal(t, 8, fullMissing.missingCount)

	fullValues := stringTemplateVarCache{valueCount: 8}
	got, ok = fullValues.resolve(vars, "flag")
	require.True(t, ok)
	assert.Equal(t, "true", got)
	assert.Equal(t, 8, fullValues.valueCount)
}

func TestMutationCoverage_WorkflowStepsTierConfigDisablesL2WhenNil(t *testing.T) {
	t.Parallel()

	cfg := workflowStepsVersionTierConfig(time.Minute, nil)
	assert.True(t, cfg.DisableL2)
	assert.Equal(t, uint32(1), cfg.Weigher(workflowStepsVersionKey{}, nil))
	assert.Equal(t, uint32(100_000), cfg.Weigher(workflowStepsVersionKey{}, make([]domain.WorkflowStep, 100_001)))
	assert.Equal(t, uint32(2), cfg.Weigher(workflowStepsVersionKey{}, make([]domain.WorkflowStep, 2)))
}
