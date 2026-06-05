package workflow

import (
	"fmt"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NotNil(t, got)

	want := start.Add(60 * time.Second)
	assert.True(t,
		got.Equal(
			want))
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
	require.NotNil(t, got)

	want := start.Add(25 * time.Second)
	assert.True(t,
		got.Equal(
			want))
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
	require.NotNil(t, got)

	want := start.Add(30 * time.Second)
	assert.True(t,
		got.Equal(
			want))
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
	require.NotNil(t, got)

	// Remaining: b(20) -> c(30) = 50s from now.
	want := now.Add(50 * time.Second)
	assert.True(t,
		got.Equal(
			want))
}

func TestExpectedCompletion_NoExpectedDurations(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a"},
		{StepRef: "b", DependsOn: []string{"a"}},
	}

	start := time.Now()
	got := CalculateExpectedCompletion(steps, start)
	assert.Nil(t, got)
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
	require.NotNil(t, got)

	want := start.Add(10 * time.Second)
	assert.True(t,
		got.Equal(
			want))
}

func TestExpectedCompletion_EmptySteps(t *testing.T) {
	t.Parallel()
	got := CalculateExpectedCompletion(nil, time.Now())
	assert.Nil(t, got)
}

func TestExpectedCompletion_SingleStep(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "only", ExpectedDurationSecs: 42},
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := CalculateExpectedCompletion(steps, start)
	require.NotNil(t, got)

	want := start.Add(42 * time.Second)
	assert.True(t,
		got.Equal(
			want))
}

func TestExpectedCompletion_DuplicateDependencyRefs(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", ExpectedDurationSecs: 10},
		{StepRef: "b", DependsOn: []string{"a", "a"}, ExpectedDurationSecs: 20},
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := CalculateExpectedCompletion(steps, start)
	require.NotNil(t, got)

	want := start.Add(30 * time.Second)
	assert.True(t,
		got.Equal(
			want))
}

func TestRecalculateExpectedCompletion_AllCompleted(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", ExpectedDurationSecs: 10},
		{StepRef: "b", DependsOn: []string{"a"}, ExpectedDurationSecs: 20},
	}

	completed := map[string]bool{"a": true, "b": true}
	got := RecalculateExpectedCompletion(steps, completed, time.Now())
	assert.Nil(t, got)
}

func TestRecalculateExpectedCompletion_CompletedParentsUnblockRemainingDAG(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "a", ExpectedDurationSecs: 5},
		{StepRef: "b", DependsOn: []string{"a"}, ExpectedDurationSecs: 10},
		{StepRef: "c", DependsOn: []string{"a"}, ExpectedDurationSecs: 20},
	}

	now := time.Date(2026, 1, 1, 0, 0, 5, 0, time.UTC)
	completed := map[string]bool{"a": true}
	got := RecalculateExpectedCompletion(steps, completed, now)
	require.NotNil(t, got)

	want := now.Add(20 * time.Second)
	assert.True(t,
		got.Equal(
			want))
}

func TestExpectedCompletion_UnorderedDefinitionsUseTopologicalFallback(t *testing.T) {
	t.Parallel()
	steps := []domain.WorkflowStep{
		{StepRef: "b", DependsOn: []string{"a"}, ExpectedDurationSecs: 20},
		{StepRef: "a", ExpectedDurationSecs: 10},
		{StepRef: "c", DependsOn: []string{"b"}, ExpectedDurationSecs: 30},
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := CalculateExpectedCompletion(steps, start)
	require.NotNil(t, got)

	want := start.Add(60 * time.Second)
	assert.True(t,
		got.Equal(
			want))
}

func TestRecalculateExpectedCompletion_LargeCompletedPrefix(t *testing.T) {
	t.Parallel()
	steps := expectedCompletionChain(1000)
	completed := make(map[string]bool, 600)
	for i := range 600 {
		completed[steps[i].StepRef] = true
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := RecalculateExpectedCompletion(steps, completed, now)
	require.NotNil(t, got)

	want := now.Add(400 * time.Second)
	assert.True(t,
		got.Equal(
			want))
}

func TestRecalculateExpectedCompletion_NonPrefixCompletionUsesFallback(t *testing.T) {
	t.Parallel()
	steps := expectedCompletionChain(5)
	completed := map[string]bool{
		steps[1].StepRef: true,
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := RecalculateExpectedCompletion(steps, completed, now)
	require.NotNil(t, got)

	want := now.Add(3 * time.Second)
	assert.True(t,
		got.Equal(
			want))
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
	require.NotNil(t, got)
	assert.False(t,
		got.Before(start),
	)

	// Should not panic, just produce a far-future time.
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
	assert.Nil(t, got)
}

func TestExpectedCompletion_1000StepWorkflow(t *testing.T) {
	t.Parallel()
	steps := expectedCompletionChain(1000)

	start := time.Now()
	got := CalculateExpectedCompletion(steps, start)
	require.NotNil(t, got)

	want := start.Add(1000 * time.Second)
	assert.True(t,
		got.Equal(
			want))
}

func BenchmarkCalculateExpectedCompletion(b *testing.B) {
	steps := expectedCompletionChain(100)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()
	for b.Loop() {
		_ = CalculateExpectedCompletion(steps, start)
	}
}

func BenchmarkCalculateExpectedCompletion_Chain1000(b *testing.B) {
	steps := expectedCompletionChain(1000)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()
	for b.Loop() {
		got := CalculateExpectedCompletion(steps, start)
		if got == nil {
			b.Fatal("expected non-nil result")
		}
	}
}

func BenchmarkRecalculateExpectedCompletion_PartialChain100(b *testing.B) {
	steps := expectedCompletionChain(100)
	completed := make(map[string]bool, 50)
	for i := range 50 {
		completed[steps[i].StepRef] = true
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()
	for b.Loop() {
		_ = RecalculateExpectedCompletion(steps, completed, now)
	}
}

func BenchmarkRecalculateExpectedCompletion_PartialChain1000(b *testing.B) {
	steps := expectedCompletionChain(1000)
	completed := make(map[string]bool, 500)
	for i := range 500 {
		completed[steps[i].StepRef] = true
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()
	for b.Loop() {
		got := RecalculateExpectedCompletion(steps, completed, now)
		if got == nil {
			b.Fatal("expected non-nil result")
		}
	}
}

func expectedCompletionChain(size int) []domain.WorkflowStep {
	steps := make([]domain.WorkflowStep, size)
	for i := range steps {
		steps[i] = domain.WorkflowStep{
			StepRef:              fmt.Sprintf("step-%04d", i),
			ExpectedDurationSecs: 1,
		}
		if i > 0 {
			steps[i].DependsOn = []string{steps[i-1].StepRef}
		}
	}
	return steps
}
