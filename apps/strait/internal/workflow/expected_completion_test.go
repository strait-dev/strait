package workflow

import (
	"testing"
	"time"

	"strait/internal/domain"
)

func TestExpectedCompletion_LinearDAG(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", ExpectedDurationSecs: 10},
		{StepRef: "b", DependsOn: []string{"a"}, ExpectedDurationSecs: 20},
		{StepRef: "c", DependsOn: []string{"b"}, ExpectedDurationSecs: 30},
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := CalculateExpectedCompletion(steps, start)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	want := start.Add(60 * time.Second)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpectedCompletion_ParallelDAG(t *testing.T) {
	t.Parallel()
	// A -> B (10s), A -> C (20s): critical path is A + C = 5 + 20 = 25s.
	steps := []domain.WorkflowStep{
		{StepRef: "a", ExpectedDurationSecs: 5},
		{StepRef: "b", DependsOn: []string{"a"}, ExpectedDurationSecs: 10},
		{StepRef: "c", DependsOn: []string{"a"}, ExpectedDurationSecs: 20},
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := CalculateExpectedCompletion(steps, start)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	want := start.Add(25 * time.Second)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpectedCompletion_DiamondDAG(t *testing.T) {
	t.Parallel()
	// A(5) -> B(10), A(5) -> C(20) -> D(5). Critical path: A->C->D = 30s.
	steps := []domain.WorkflowStep{
		{StepRef: "a", ExpectedDurationSecs: 5},
		{StepRef: "b", DependsOn: []string{"a"}, ExpectedDurationSecs: 10},
		{StepRef: "c", DependsOn: []string{"a"}, ExpectedDurationSecs: 20},
		{StepRef: "d", DependsOn: []string{"b", "c"}, ExpectedDurationSecs: 5},
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := CalculateExpectedCompletion(steps, start)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	want := start.Add(30 * time.Second)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpectedCompletion_Recalculation(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", ExpectedDurationSecs: 10},
		{StepRef: "b", DependsOn: []string{"a"}, ExpectedDurationSecs: 20},
		{StepRef: "c", DependsOn: []string{"b"}, ExpectedDurationSecs: 30},
	}

	now := time.Date(2026, 1, 1, 0, 0, 15, 0, time.UTC)
	completed := map[string]bool{"a": true}

	got := RecalculateExpectedCompletion(steps, completed, now)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	// Remaining: b(20) -> c(30) = 50s from now.
	want := now.Add(50 * time.Second)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpectedCompletion_NoExpectedDurations(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a"},
		{StepRef: "b", DependsOn: []string{"a"}},
	}

	start := time.Now()
	got := CalculateExpectedCompletion(steps, start)
	if got != nil {
		t.Errorf("expected nil when no durations configured, got %v", got)
	}
}

func TestExpectedCompletion_MixedDurations(t *testing.T) {
	t.Parallel()
	// Only step a has duration. b has 0.
	steps := []domain.WorkflowStep{
		{StepRef: "a", ExpectedDurationSecs: 10},
		{StepRef: "b", DependsOn: []string{"a"}, ExpectedDurationSecs: 0},
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := CalculateExpectedCompletion(steps, start)
	if got == nil {
		t.Fatal("expected non-nil when at least one step has duration")
	}
	want := start.Add(10 * time.Second)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpectedCompletion_EmptySteps(t *testing.T) {
	t.Parallel()
	got := CalculateExpectedCompletion(nil, time.Now())
	if got != nil {
		t.Errorf("expected nil for empty steps, got %v", got)
	}
}

func TestExpectedCompletion_SingleStep(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "only", ExpectedDurationSecs: 42},
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := CalculateExpectedCompletion(steps, start)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	want := start.Add(42 * time.Second)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpectedCompletion_DuplicateDependencyRefs(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", ExpectedDurationSecs: 10},
		{StepRef: "b", DependsOn: []string{"a", "a"}, ExpectedDurationSecs: 20},
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := CalculateExpectedCompletion(steps, start)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	want := start.Add(30 * time.Second)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRecalculateExpectedCompletion_AllCompleted(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", ExpectedDurationSecs: 10},
		{StepRef: "b", DependsOn: []string{"a"}, ExpectedDurationSecs: 20},
	}

	completed := map[string]bool{"a": true, "b": true}
	got := RecalculateExpectedCompletion(steps, completed, time.Now())
	if got != nil {
		t.Errorf("expected nil when all completed, got %v", got)
	}
}

// Fuzz tests for expected completion.

func FuzzExpectedCompletionCalculation(f *testing.F) {
	f.Add(uint8(3), "a,b,c", ",a,b", 10, 20, 30)
	f.Add(uint8(1), "x", "", 5, 0, 0)
	f.Add(uint8(5), "a,b,c,d,e", ",a,a,b,c", 1, 2, 3)

	f.Fuzz(func(t *testing.T, numSteps uint8, refsCSV, depsCSV string, d1, d2, d3 int) {
		if numSteps == 0 || numSteps > 20 {
			return
		}

		refs := splitComma(refsCSV)
		deps := splitComma(depsCSV)
		durations := []int{d1, d2, d3}

		if len(refs) < int(numSteps) {
			return
		}

		steps := make([]domain.WorkflowStep, numSteps)
		for i := range steps {
			steps[i].StepRef = refs[i]
			if i < len(deps) && deps[i] != "" {
				steps[i].DependsOn = []string{deps[i]}
			}
			if i < len(durations) && durations[i] >= 0 {
				steps[i].ExpectedDurationSecs = durations[i]
			}
		}

		// Must never panic.
		_ = CalculateExpectedCompletion(steps, time.Now())
	})
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	result := []string{}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	return result
}

// Adversarial tests.

func TestExpectedCompletion_MaxIntDuration(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", ExpectedDurationSecs: 1<<31 - 1},
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := CalculateExpectedCompletion(steps, start)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	// Should not panic, just produce a far-future time.
	if got.Before(start) {
		t.Error("expected completion should be after start")
	}
}

func TestExpectedCompletion_ZeroDuration(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", ExpectedDurationSecs: 0},
		{StepRef: "b", DependsOn: []string{"a"}, ExpectedDurationSecs: 0},
	}

	// All zero durations, has steps with ExpectedDurationSecs but all are 0.
	// hasAny check looks for > 0, so this should return nil.
	got := CalculateExpectedCompletion(steps, time.Now())
	if got != nil {
		t.Errorf("expected nil for all-zero durations, got %v", got)
	}
}

func TestExpectedCompletion_1000StepWorkflow(t *testing.T) {
	t.Parallel()
	steps := make([]domain.WorkflowStep, 1000)
	steps[0] = domain.WorkflowStep{StepRef: "s0", ExpectedDurationSecs: 1}
	for i := 1; i < 1000; i++ {
		steps[i] = domain.WorkflowStep{
			StepRef:              "s" + string(rune('0'+i%10)) + string(rune('0'+i/10%10)) + string(rune('0'+i/100%10)),
			DependsOn:            []string{steps[i-1].StepRef},
			ExpectedDurationSecs: 1,
		}
	}

	start := time.Now()
	got := CalculateExpectedCompletion(steps, start)
	if got == nil {
		t.Fatal("expected non-nil for 1000-step chain")
	}
	want := start.Add(1000 * time.Second)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func BenchmarkCalculateExpectedCompletion(b *testing.B) {
	steps := make([]domain.WorkflowStep, 100)
	for i := range steps {
		steps[i] = domain.WorkflowStep{
			StepRef:              "step-" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			ExpectedDurationSecs: 1,
		}
		if i > 0 {
			steps[i].DependsOn = []string{steps[i-1].StepRef}
		}
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()
	for b.Loop() {
		_ = CalculateExpectedCompletion(steps, start)
	}
}
