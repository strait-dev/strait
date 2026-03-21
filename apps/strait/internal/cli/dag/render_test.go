package dag

import (
	"strings"
	"testing"
)

func TestRenderDAG_SingleNode(t *testing.T) {
	t.Parallel()

	result := RenderDAG([]Step{{StepRef: "a"}}, nil)
	if !strings.Contains(result, "a") {
		t.Fatalf("expected 'a' in output, got:\n%s", result)
	}
}

func TestRenderDAG_LinearChain(t *testing.T) {
	t.Parallel()

	steps := []Step{
		{StepRef: "a"},
		{StepRef: "b", DependsOn: []string{"a"}},
		{StepRef: "c", DependsOn: []string{"b"}},
	}
	result := RenderDAG(steps, nil)
	aIdx := strings.Index(result, "a")
	bIdx := strings.Index(result, "b")
	cIdx := strings.Index(result, "c")
	if aIdx >= bIdx || bIdx >= cIdx {
		t.Fatalf("expected a before b before c in:\n%s", result)
	}
}

func TestRenderDAG_ParallelBranches(t *testing.T) {
	t.Parallel()

	steps := []Step{
		{StepRef: "a"},
		{StepRef: "b", DependsOn: []string{"a"}},
		{StepRef: "c", DependsOn: []string{"a"}},
		{StepRef: "d", DependsOn: []string{"b", "c"}},
	}
	result := RenderDAG(steps, nil)
	if !strings.Contains(result, "a") || !strings.Contains(result, "b") || !strings.Contains(result, "c") || !strings.Contains(result, "d") {
		t.Fatalf("expected all nodes in output:\n%s", result)
	}
}

func TestRenderDAG_CycleDetection(t *testing.T) {
	t.Parallel()

	steps := []Step{
		{StepRef: "a", DependsOn: []string{"b"}},
		{StepRef: "b", DependsOn: []string{"a"}},
	}
	result := RenderDAG(steps, nil)
	if !strings.Contains(result, "cycle detected") {
		t.Fatalf("expected 'cycle detected', got:\n%s", result)
	}
}

func TestRenderDAG_EmptySteps(t *testing.T) {
	t.Parallel()

	result := RenderDAG([]Step{}, nil)
	if result != "(empty workflow)" {
		t.Fatalf("expected '(empty workflow)', got: %q", result)
	}
}

func TestRenderDAG_DanglingDependsOn(t *testing.T) {
	t.Parallel()

	steps := []Step{
		{StepRef: "b", DependsOn: []string{"nonexistent"}},
	}
	result := RenderDAG(steps, nil)
	if !strings.Contains(result, "unknown step") {
		t.Fatalf("expected 'unknown step' for dangling dependency, got:\n%s", result)
	}
	if strings.Contains(result, "cycle") {
		t.Fatalf("should not report cycle for dangling dependency:\n%s", result)
	}
}

func TestRenderDAG_WithStatusCompleted(t *testing.T) {
	t.Parallel()

	steps := []Step{{StepRef: "a"}}
	statusMap := map[string]string{"a": "completed"}
	result := RenderDAG(steps, statusMap)
	if !strings.Contains(result, "\033[32m") {
		t.Fatalf("expected green ANSI code for completed status:\n%s", result)
	}
}

func TestRenderDAG_WithStatusFailed(t *testing.T) {
	t.Parallel()

	steps := []Step{{StepRef: "a"}}
	statusMap := map[string]string{"a": "failed"}
	result := RenderDAG(steps, statusMap)
	if !strings.Contains(result, "\033[31m") {
		t.Fatalf("expected red ANSI code for failed status:\n%s", result)
	}
}

func TestRenderDAG_NilStatusMap(t *testing.T) {
	t.Parallel()

	steps := []Step{{StepRef: "a"}}
	result := RenderDAG(steps, nil)
	if strings.Contains(result, "\033[") {
		t.Fatalf("expected no ANSI codes with nil statusMap:\n%s", result)
	}
	if !strings.Contains(result, "a") {
		t.Fatalf("expected 'a' in output:\n%s", result)
	}
}

func TestRenderDAG_DuplicateStepRef(t *testing.T) {
	t.Parallel()

	steps := []Step{
		{StepRef: "a", DependsOn: []string{}},
		{StepRef: "a", DependsOn: []string{}},
	}
	// Should not panic; second overwrites first
	result := RenderDAG(steps, nil)
	if !strings.Contains(result, "a") {
		t.Fatalf("expected 'a' in output:\n%s", result)
	}
}
