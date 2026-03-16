package authoring

import (
	"errors"
	"testing"

	strait "github.com/strait-dev/go-sdk"
)

func TestValidateDag_Empty(t *testing.T) {
	sorted, err := ValidateDag(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sorted != nil {
		t.Error("expected nil for empty DAG")
	}
}

func TestValidateDag_LinearChain(t *testing.T) {
	steps := []Step{
		Job("a", "job_1"),
		Job("b", "job_2", DependsOn("a")),
		Job("c", "job_3", DependsOn("b")),
	}

	sorted, err := ValidateDag(steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorted) != 3 {
		t.Errorf("expected 3 sorted refs, got %d", len(sorted))
	}
	if sorted[0] != "a" || sorted[1] != "b" || sorted[2] != "c" {
		t.Errorf("expected [a, b, c], got %v", sorted)
	}
}

func TestValidateDag_Diamond(t *testing.T) {
	steps := []Step{
		Job("a", "job_1"),
		Job("b", "job_2", DependsOn("a")),
		Job("c", "job_3", DependsOn("a")),
		Job("d", "job_4", DependsOn("b", "c")),
	}

	sorted, err := ValidateDag(steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorted) != 4 {
		t.Errorf("expected 4 sorted refs, got %d", len(sorted))
	}
	if sorted[0] != "a" {
		t.Errorf("expected 'a' first, got %q", sorted[0])
	}
}

func TestValidateDag_ParallelSteps(t *testing.T) {
	steps := []Step{
		Job("a", "job_1"),
		Job("b", "job_2"),
		Job("c", "job_3"),
	}

	sorted, err := ValidateDag(steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorted) != 3 {
		t.Errorf("expected 3 sorted refs, got %d", len(sorted))
	}
}

func TestValidateDag_CycleDetection(t *testing.T) {
	steps := []Step{
		Job("a", "job_1", DependsOn("c")),
		Job("b", "job_2", DependsOn("a")),
		Job("c", "job_3", DependsOn("b")),
	}

	_, err := ValidateDag(steps)
	if err == nil {
		t.Fatal("expected error for cycle")
	}
	var dve *strait.DagValidationError
	if !errors.As(err, &dve) {
		t.Errorf("expected DagValidationError, got %T", err)
	}
	if len(dve.Cycles) == 0 {
		t.Error("expected cycles to be reported")
	}
}

func TestValidateDag_MissingRef(t *testing.T) {
	steps := []Step{
		Job("a", "job_1"),
		Job("b", "job_2", DependsOn("nonexistent")),
	}

	_, err := ValidateDag(steps)
	if err == nil {
		t.Fatal("expected error for missing ref")
	}
	var dve *strait.DagValidationError
	if !errors.As(err, &dve) {
		t.Errorf("expected DagValidationError, got %T", err)
	}
	if len(dve.MissingRefs) == 0 {
		t.Error("expected missing refs to be reported")
	}
}

func TestValidateDag_DuplicateRef(t *testing.T) {
	steps := []Step{
		Job("a", "job_1"),
		Job("a", "job_2"),
	}

	_, err := ValidateDag(steps)
	if err == nil {
		t.Fatal("expected error for duplicate ref")
	}
	var dve *strait.DagValidationError
	if !errors.As(err, &dve) {
		t.Errorf("expected DagValidationError, got %T", err)
	}
	if len(dve.DuplicateRefs) == 0 {
		t.Error("expected duplicate refs to be reported")
	}
}

func TestValidateDag_MixedStepTypes(t *testing.T) {
	steps := []Step{
		Job("validate", "job_validate"),
		Approval("review", func(a *ApprovalStep) {
			a.DependsOn = []string{"validate"}
		}),
		WaitForEvent("confirm", "shipping.confirmed", func(w *WaitForEventStep) {
			w.DependsOn = []string{"review"}
		}),
		Sleep("cooldown", 60, DependsOn("confirm")),
		SubWorkflow("notify", "wf_notifications", func(sw *SubWorkflowStep) {
			sw.DependsOn = []string{"cooldown"}
		}),
	}

	sorted, err := ValidateDag(steps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorted) != 5 {
		t.Errorf("expected 5 sorted refs, got %d", len(sorted))
	}
}
