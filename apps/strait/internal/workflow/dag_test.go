package workflow

import (
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestValidateDAG(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		steps   []domain.WorkflowStep
		wantErr string
	}{
		{
			name:  "valid: single step, no deps",
			steps: []domain.WorkflowStep{step("A")},
		},
		{
			name: "valid: linear chain A->B->C",
			steps: []domain.WorkflowStep{
				step("A"),
				step("B", "A"),
				step("C", "B"),
			},
		},
		{
			name: "valid: diamond DAG (fan-out + fan-in)",
			steps: []domain.WorkflowStep{
				step("A"),
				step("B", "A"),
				step("C", "A"),
				step("D", "B", "C"),
			},
		},
		{
			name: "valid: wide fan-out",
			steps: []domain.WorkflowStep{
				step("A"),
				step("B", "A"),
				step("C", "A"),
				step("D", "A"),
				step("E", "A"),
			},
		},
		{
			name: "valid: complex DAG",
			steps: []domain.WorkflowStep{
				step("A"),
				step("B", "A"),
				step("C", "A"),
				step("D", "B"),
				step("E", "B", "C"),
				step("F", "D", "E"),
				step("G", "C"),
			},
		},
		{
			name:    "error: empty steps",
			steps:   nil,
			wantErr: "at least one step",
		},
		{
			name: "error: duplicate step_ref",
			steps: []domain.WorkflowStep{
				step("A"),
				step("A"),
			},
			wantErr: "duplicate step_ref",
		},
		{
			name: "error: unknown dependency",
			steps: []domain.WorkflowStep{
				step("A"),
				step("B", "missing"),
			},
			wantErr: "unknown step",
		},
		{
			name: "error: self-dependency",
			steps: []domain.WorkflowStep{
				step("A", "A"),
			},
			wantErr: "depends on itself",
		},
		{
			name: "error: simple cycle A->B->A",
			steps: []domain.WorkflowStep{
				step("A", "B"),
				step("B", "A"),
			},
			wantErr: "cycle detected",
		},
		{
			name: "error: three-node cycle A->B->C->A",
			steps: []domain.WorkflowStep{
				step("A", "C"),
				step("B", "A"),
				step("C", "B"),
			},
			wantErr: "cycle detected",
		},
		{
			name: "error: cycle in subgraph",
			steps: []domain.WorkflowStep{
				step("A"),
				step("B", "A"),
				step("C", "D"),
				step("D", "C"),
			},
			wantErr: "cycle detected",
		},
		{
			name: "error: all steps depend on each other",
			steps: []domain.WorkflowStep{
				step("A", "B", "C"),
				step("B", "A", "C"),
				step("C", "A", "B"),
			},
			wantErr: "cycle detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateDAG(tt.steps)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateDAG() unexpected error: %v", err)
				}
				return
			}

			assertErrorContains(t, err, tt.wantErr)
		})
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Errorf("expected error containing %q, got nil", want)
		return
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err.Error(), want)
	}
}

func step(ref string, deps ...string) domain.WorkflowStep {
	return domain.WorkflowStep{
		StepRef:   ref,
		DependsOn: deps,
	}
}

func BenchmarkValidateDAG(b *testing.B) {
	steps := make([]domain.WorkflowStep, 20)
	for i := range steps {
		steps[i] = domain.WorkflowStep{StepRef: strings.Repeat("s", i+1)}
		if i > 0 {
			steps[i].DependsOn = []string{steps[i-1].StepRef}
		}
	}
	for b.Loop() {
		_ = ValidateDAG(steps)
	}
}
