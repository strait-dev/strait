package queue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectAllowlist_Empty(t *testing.T) {
	a := NewProjectLabelAllowlist(10)

	assert.Equal(t, "_other", a.Label("p1"))
}

func TestProjectAllowlist_SetAddList(t *testing.T) {
	a := NewProjectLabelAllowlist(5)
	a.Set([]string{"p1", "p2", "p3"})

	assert.EqualValues(t, 3, a.Size())
	assert.Equal(t, "p1", a.Label("p1"))
	assert.Equal(t, "_other", a.Label("p999"))
}

func TestProjectAllowlist_SetRespectsCap(t *testing.T) {
	// Max 5 ⇒ 4 real + 1 fallback.
	a := NewProjectLabelAllowlist(5)
	a.Set([]string{"a", "b", "c", "d", "e", "f"})

	assert.EqualValues(t, 4, a.Size())
}

func TestProjectAllowlist_AddOverflow(t *testing.T) {
	a := NewProjectLabelAllowlist(3) // 2 real slots
	a.Add("p1")
	a.Add("p2")

	assert.False(t, a.Add("p3"))
}

func TestProjectAllowlist_AddIdempotent(t *testing.T) {
	a := NewProjectLabelAllowlist(3)
	a.Add("p1")
	a.Add("p1")

	assert.EqualValues(t, 1, a.Size())
}

func TestProjectAllowlist_LargeWorkloadBoundedCardinality(t *testing.T) {
	a := NewProjectLabelAllowlist(51) // 50 real slots + fallback
	for i := range 500 {
		a.Add("project-" + string(rune('a'+(i%26))))
	}

	assert.LessOrEqual(t, a.Size(), 50)
}

func TestRecordClaimLatencyByProject_NilSafe(t *testing.T) {
	var m *QueueMetrics
	// Must not panic.
	m.RecordClaimLatencyByProject(context.Background(), nil, "p1", 1.5)
}

func TestRecordClaimLatencyByProject_WithoutAllowlist(t *testing.T) {
	m, err := Metrics()
	require.NoError(t, err)

	// No allowlist ⇒ should fall back to label-less path.
	m.RecordClaimLatencyByProject(context.Background(), nil, "p1", 0.1)
}

func TestRecordClaimLatencyByProject_WithAllowlist(t *testing.T) {
	m, err := Metrics()
	require.NoError(t, err)

	a := NewProjectLabelAllowlist(10)
	a.Set([]string{"p1", "p2"})
	m.RecordClaimLatencyByProject(context.Background(), a, "p1", 0.1)
	m.RecordClaimLatencyByProject(context.Background(), a, "p999", 0.2) // _other
}
