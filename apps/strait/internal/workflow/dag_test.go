package workflow

import (
	"fmt"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			name: "valid: duplicate dependency refs are deduplicated",
			steps: []domain.WorkflowStep{
				step("A"),
				step("B", "A", "A"),
			},
		},
		{
			name: "valid: unordered input falls back to topological validation",
			steps: []domain.WorkflowStep{
				step("B", "A"),
				step("A"),
			},
		},
		{
			name: "valid: unordered input with duplicate deps falls back to topological validation",
			steps: []domain.WorkflowStep{
				step("B", "A", "A"),
				step("A"),
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
				assert.NoError(t, err)

				return
			}

			assertErrorContains(t, err, tt.wantErr)
		})
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	require.Error(t,
		err)
	assert.Contains(t,
		err.
			Error(), want)
}

func step(ref string, deps ...string) domain.WorkflowStep {
	return domain.WorkflowStep{
		StepRef:   ref,
		DependsOn: deps,
	}
}

func BenchmarkValidateDAG(b *testing.B) {
	steps := benchmarkDAGChain(20)
	for b.Loop() {
		_ = ValidateDAG(steps)
	}
}

func BenchmarkValidateDAG_Chain1000(b *testing.B) {
	steps := benchmarkDAGChain(1000)
	b.ReportAllocs()
	for b.Loop() {
		if err := ValidateDAG(steps); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildTopologicalOrder(b *testing.B) {
	steps := benchmarkDAGChain(100)
	b.ReportAllocs()
	for b.Loop() {
		_ = buildTopologicalOrder(steps)
	}
}

func BenchmarkBuildTopologicalOrder_Chain1000(b *testing.B) {
	steps := benchmarkDAGChain(1000)
	b.ReportAllocs()
	for b.Loop() {
		order := buildTopologicalOrder(steps)
		if len(order) != len(steps) {
			b.Fatalf("order length = %d, want %d", len(order), len(steps))
		}
	}
}

func benchmarkDAGChain(size int) []domain.WorkflowStep {
	steps := make([]domain.WorkflowStep, size)
	for i := range steps {
		steps[i] = domain.WorkflowStep{StepRef: fmt.Sprintf("step-%04d", i)}
		if i > 0 {
			steps[i].DependsOn = []string{steps[i-1].StepRef}
		}
	}
	return steps
}
