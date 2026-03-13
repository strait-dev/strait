package workflow

import (
	"encoding/json"
	"strings"
	"testing"

	"strait/internal/domain"
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
			name:         "not: inverts nested result",
			cond:         mustJSON(`{"type":"not","condition":{"type":"step_status","step_ref":"validate-data","status":"failed"}}`),
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := EvaluateCondition(tt.cond, tt.stepStatuses)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("EvaluateCondition() = %v, want %v", got, tt.want)
			}
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
	composite := json.RawMessage(`{"type":"all_of","conditions":[{"type":"step_status","step_ref":"step-1","status":"completed"},{"type":"step_status","step_ref":"step-2","status":"completed"},{"type":"step_status","step_ref":"step-3","status":"failed"}]}`)

	b.Run("simple", func(b *testing.B) {
		for range b.N {
			_, _ = EvaluateCondition(simple, statuses)
		}
	})
	b.Run("composite", func(b *testing.B) {
		for range b.N {
			_, _ = EvaluateCondition(composite, statuses)
		}
	})
}

func mustJSON(s string) json.RawMessage {
	return json.RawMessage(s)
}
