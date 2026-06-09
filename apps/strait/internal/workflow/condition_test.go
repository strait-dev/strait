package workflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateCondition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		cond         json.RawMessage
		stepStatuses map[string]domain.StepRunStatus
		want         bool
		wantErr      bool
		errContains  string
	}{
		{
			name:         "nil condition -> true",
			cond:         nil,
			stepStatuses: map[string]domain.StepRunStatus{},
			want:         true,
		},
		{
			name:         "empty condition -> true",
			cond:         json.RawMessage{},
			stepStatuses: map[string]domain.StepRunStatus{},
			want:         true,
		},
		{
			name: "step_status: completed match -> true",
			cond: mustJSON(`{"type":"step_status","step_ref":"validate-data","status":"completed"}`),
			stepStatuses: map[string]domain.StepRunStatus{
				"validate-data": domain.StepCompleted,
			},
			want: true,
		},
		{
			name: "step_status: completed mismatch -> false",
			cond: mustJSON(`{"type":"step_status","step_ref":"validate-data","status":"completed"}`),
			stepStatuses: map[string]domain.StepRunStatus{
				"validate-data": domain.StepFailed,
			},
			want: false,
		},
		{
			name: "step_status: failed match -> true",
			cond: mustJSON(`{"type":"step_status","step_ref":"validate-data","status":"failed"}`),
			stepStatuses: map[string]domain.StepRunStatus{
				"validate-data": domain.StepFailed,
			},
			want: true,
		},
		{
			name:         "step_status: unknown step_ref -> error",
			cond:         mustJSON(`{"type":"step_status","step_ref":"missing-step","status":"completed"}`),
			stepStatuses: map[string]domain.StepRunStatus{},
			wantErr:      true,
			errContains:  `step "missing-step" not found in statuses`,
		},
		{
			name: "all_of: all true -> true",
			cond: mustJSON(`{
				"type":"all_of",
				"conditions":[
					{"type":"step_status","step_ref":"validate-data","status":"completed"},
					{"type":"step_status","step_ref":"prepare-input","status":"failed"}
				]
			}`),
			stepStatuses: map[string]domain.StepRunStatus{
				"validate-data": domain.StepCompleted,
				"prepare-input": domain.StepFailed,
			},
			want: true,
		},
		{
			name: "all_of: one false -> false",
			cond: mustJSON(`{
				"type":"all_of",
				"conditions":[
					{"type":"step_status","step_ref":"validate-data","status":"completed"},
					{"type":"step_status","step_ref":"prepare-input","status":"failed"}
				]
			}`),
			stepStatuses: map[string]domain.StepRunStatus{
				"validate-data": domain.StepCompleted,
				"prepare-input": domain.StepCompleted,
			},
			want: false,
		},
		{
			name:         "all_of: empty conditions -> true",
			cond:         mustJSON(`{"type":"all_of","conditions":[]}`),
			stepStatuses: map[string]domain.StepRunStatus{},
			want:         true,
		},
		{
			name: "any_of: one true -> true",
			cond: mustJSON(`{
				"type":"any_of",
				"conditions":[
					{"type":"step_status","step_ref":"validate-data","status":"failed"},
					{"type":"step_status","step_ref":"prepare-input","status":"completed"}
				]
			}`),
			stepStatuses: map[string]domain.StepRunStatus{
				"validate-data": domain.StepCompleted,
				"prepare-input": domain.StepCompleted,
			},
			want: true,
		},
		{
			name: "any_of: none true -> false",
			cond: mustJSON(`{
				"type":"any_of",
				"conditions":[
					{"type":"step_status","step_ref":"validate-data","status":"failed"},
					{"type":"step_status","step_ref":"prepare-input","status":"failed"}
				]
			}`),
			stepStatuses: map[string]domain.StepRunStatus{
				"validate-data": domain.StepCompleted,
				"prepare-input": domain.StepCompleted,
			},
			want: false,
		},
		{
			name:         "any_of: empty conditions -> false",
			cond:         mustJSON(`{"type":"any_of","conditions":[]}`),
			stepStatuses: map[string]domain.StepRunStatus{},
			want:         false,
		},
		{
			name: "nested: all_of containing any_of -> works",
			cond: mustJSON(`{
				"type":"all_of",
				"conditions":[
					{"type":"step_status","step_ref":"validate-data","status":"completed"},
					{
						"type":"any_of",
						"conditions":[
							{"type":"step_status","step_ref":"prepare-input","status":"failed"},
							{"type":"step_status","step_ref":"send-email","status":"completed"}
						]
					}
				]
			}`),
			stepStatuses: map[string]domain.StepRunStatus{
				"validate-data": domain.StepCompleted,
				"prepare-input": domain.StepCompleted,
				"send-email":    domain.StepCompleted,
			},
			want: true,
		},
		{
			name:         "step_status_in: one allowed status -> true",
			cond:         mustJSON(`{"type":"step_status_in","step_ref":"validate-data","statuses":["failed","completed"]}`),
			stepStatuses: map[string]domain.StepRunStatus{"validate-data": domain.StepCompleted},
			want:         true,
		},
		{
			name:         "step_status_in: empty statuses -> false",
			cond:         mustJSON(`{"type":"step_status_in","step_ref":"validate-data","statuses":[]}`),
			stepStatuses: map[string]domain.StepRunStatus{"validate-data": domain.StepCompleted},
			want:         false,
		},
		{
			name:         "not: inverts nested result",
			cond:         mustJSON(`{"type":"not","condition":{"type":"step_status","step_ref":"validate-data","status":"failed"}}`),
			stepStatuses: map[string]domain.StepRunStatus{"validate-data": domain.StepCompleted},
			want:         true,
		},
		{
			name: "all_of: short-circuits false before missing step",
			cond: mustJSON(`{
				"type":"all_of",
				"conditions":[
					{"type":"step_status","step_ref":"validate-data","status":"failed"},
					{"type":"step_status","step_ref":"missing-step","status":"completed"}
				]
			}`),
			stepStatuses: map[string]domain.StepRunStatus{"validate-data": domain.StepCompleted},
			want:         false,
		},
		{
			name: "any_of: short-circuits true before missing step",
			cond: mustJSON(`{
				"type":"any_of",
				"conditions":[
					{"type":"step_status","step_ref":"validate-data","status":"completed"},
					{"type":"step_status","step_ref":"missing-step","status":"completed"}
				]
			}`),
			stepStatuses: map[string]domain.StepRunStatus{"validate-data": domain.StepCompleted},
			want:         true,
		},
		{
			name:         "eq operator",
			cond:         mustJSON(`{"type":"eq","left":"a","right":"a"}`),
			stepStatuses: map[string]domain.StepRunStatus{},
			want:         true,
		},
		{
			name:         "gt operator",
			cond:         mustJSON(`{"type":"gt","left":3,"right":2}`),
			stepStatuses: map[string]domain.StepRunStatus{},
			want:         true,
		},
		{
			name:         "regex operator",
			cond:         mustJSON(`{"type":"regex","left":"abc-123","right":"^[a-z]+-[0-9]+$"}`),
			stepStatuses: map[string]domain.StepRunStatus{},
			want:         true,
		},
		{
			name:         "exists operator with missing step_ref",
			cond:         mustJSON(`{"type":"exists","operand":{"step_ref":"missing"}}`),
			stepStatuses: map[string]domain.StepRunStatus{},
			want:         false,
		},
		{
			name:         "unknown type -> error",
			cond:         mustJSON(`{"type":"foobar"}`),
			stepStatuses: map[string]domain.StepRunStatus{},
			wantErr:      true,
			errContains:  `unknown condition type: "foobar"`,
		},
		{
			name:         "invalid JSON -> error",
			cond:         mustJSON(`{"type":`),
			stepStatuses: map[string]domain.StepRunStatus{},
			wantErr:      true,
		},
		{
			name:         "step_status: missing step_ref field -> error",
			cond:         mustJSON(`{"type":"step_status","status":"completed"}`),
			stepStatuses: map[string]domain.StepRunStatus{},
			wantErr:      true,
			errContains:  "step_ref is required",
		},
		{
			name:         "step_status: non-string step_ref -> error",
			cond:         mustJSON(`{"type":"step_status","step_ref":123,"status":"completed"}`),
			stepStatuses: map[string]domain.StepRunStatus{},
			wantErr:      true,
			errContains:  "unmarshal step_status condition",
		},
		{
			name:         "step_status_in: non-array statuses -> error",
			cond:         mustJSON(`{"type":"step_status_in","step_ref":"validate-data","statuses":{}}`),
			stepStatuses: map[string]domain.StepRunStatus{"validate-data": domain.StepCompleted},
			wantErr:      true,
			errContains:  "unmarshal step_status_in condition",
		},
		{
			name:         "all_of: non-array conditions -> error",
			cond:         mustJSON(`{"type":"all_of","conditions":{}}`),
			stepStatuses: map[string]domain.StepRunStatus{},
			wantErr:      true,
			errContains:  "unmarshal all_of condition",
		},
		{
			name:         "all_of: nested non-string step_ref -> error",
			cond:         mustJSON(`{"type":"all_of","conditions":[{"type":"step_status","step_ref":123,"status":"completed"}]}`),
			stepStatuses: map[string]domain.StepRunStatus{},
			wantErr:      true,
			errContains:  "unmarshal step_status condition",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := EvaluateCondition(tt.cond, tt.stepStatuses)

			if tt.wantErr {
				require.Error(t, err)
				require.False(t, tt.errContains !=
					"" && !strings.Contains(err.Error(), tt.errContains))

				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want,
				got)
		})
	}
}

func BenchmarkEvaluateCondition(b *testing.B) {
	statuses := map[string]domain.StepRunStatus{
		"step-1": domain.StepCompleted,
		"step-2": domain.StepCompleted,
		"step-3": domain.StepFailed,
	}

	simple := json.RawMessage(`{"type":"step_status","step_ref":"step-1","status":"completed"}`)
	statusIn := json.RawMessage(`{"type":"step_status_in","step_ref":"step-3","statuses":["failed","timed_out"]}`)
	negated := json.RawMessage(`{"type":"not","condition":{"type":"step_status","step_ref":"step-3","status":"completed"}}`)
	composite := json.RawMessage(`{"type":"all_of","conditions":[{"type":"step_status","step_ref":"step-1","status":"completed"},{"type":"step_status","step_ref":"step-2","status":"completed"},{"type":"step_status","step_ref":"step-3","status":"failed"}]}`)
	eq := json.RawMessage(`{"type":"eq","left":{"step_ref":"step-1"},"right":"completed"}`)
	regex := json.RawMessage(`{"type":"regex","left":"worker-heartbeat-0123","right":"^worker-[a-z]+-[0-9]+$"}`)

	b.Run("simple", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = EvaluateCondition(simple, statuses)
		}
	})
	b.Run("status_in", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = EvaluateCondition(statusIn, statuses)
		}
	})
	b.Run("not", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = EvaluateCondition(negated, statuses)
		}
	})
	b.Run("composite", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = EvaluateCondition(composite, statuses)
		}
	})
	b.Run("eq_status_ref", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = EvaluateCondition(eq, statuses)
		}
	})
	b.Run("regex", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = EvaluateCondition(regex, statuses)
		}
	})
}

func mustJSON(s string) json.RawMessage {
	return json.RawMessage(s)
}

func TestCondition_Regex_PatternLengthLimit(t *testing.T) {
	t.Parallel()

	// Build a pattern that exceeds the max length.
	longPattern := strings.Repeat("a", maxRegexPatternLen+1)
	cond := mustJSON(`{"type":"regex","left":"hello","right":"` + longPattern + `"}`)

	_, err := EvaluateCondition(cond, map[string]domain.StepRunStatus{})
	require.Error(t, err)
	assert.Contains(t, err.
		Error(), "exceeds maximum length")
}

func TestCondition_Regex_InputLengthLimit(t *testing.T) {
	t.Parallel()

	// Normal pattern, oversized input.
	longInput := strings.Repeat("a", maxRegexInputLen+1)
	cond := mustJSON(`{"type":"regex","left":"` + longInput + `","right":"a+"}`)

	_, err := EvaluateCondition(cond, map[string]domain.StepRunStatus{})
	require.Error(t, err)
	assert.Contains(t, err.
		Error(), "exceeds maximum length")
}

func TestCondition_Regex_ReDoS_DoesNotHang(t *testing.T) {
	var concWG conc.WaitGroup

	// Note: Go's regexp engine (RE2-based) does not backtrack, so this pattern
	// won't cause catastrophic backtracking per se. The length limits are the
	// primary defense against adversarial inputs.
	defer concWG.Wait()
	t.Parallel()

	cond := mustJSON(`{"type":"regex","left":"` + strings.Repeat("a", 100) + `","right":"(a+)+b"}`)

	done := make(chan struct{})
	concWG.Go(func() {
		_, _ = EvaluateCondition(cond, map[string]domain.StepRunStatus{})
		close(done)
	})

	select {
	case <-done:
		// completed without hanging
	case <-time.After(2 * time.Second):
		require.Fail(t, "regex evaluation appears to hang (possible ReDoS)")
	}
}

func TestCondition_Regex_ValidPattern(t *testing.T) {
	t.Parallel()

	cond := mustJSON(`{"type":"regex","left":"hello-world-123","right":"^hello-.*-\\d+$"}`)
	got, err := EvaluateCondition(cond, map[string]domain.StepRunStatus{})
	require.NoError(t, err)
	assert.True(t, got)
}
