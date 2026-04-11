package queue

import (
	"context"
	"testing"
)

func TestProjectAllowlist_Empty(t *testing.T) {
	a := NewProjectLabelAllowlist(10)
	if a.Label("p1") != "_other" {
		t.Error("empty allowlist should yield _other")
	}
}

func TestProjectAllowlist_SetAddList(t *testing.T) {
	a := NewProjectLabelAllowlist(5)
	a.Set([]string{"p1", "p2", "p3"})
	if a.Size() != 3 {
		t.Errorf("size = %d", a.Size())
	}
	if a.Label("p1") != "p1" {
		t.Errorf("p1 not allowed")
	}
	if a.Label("p999") != "_other" {
		t.Errorf("unknown should be _other")
	}
}

func TestProjectAllowlist_SetRespectsCap(t *testing.T) {
	// Max 5 ⇒ 4 real + 1 fallback.
	a := NewProjectLabelAllowlist(5)
	a.Set([]string{"a", "b", "c", "d", "e", "f"})
	if a.Size() != 4 {
		t.Errorf("size = %d, want 4 (reserved fallback slot)", a.Size())
	}
}

func TestProjectAllowlist_AddOverflow(t *testing.T) {
	a := NewProjectLabelAllowlist(3) // 2 real slots
	a.Add("p1")
	a.Add("p2")
	if ok := a.Add("p3"); ok {
		t.Error("Add past cap should return false")
	}
}

func TestProjectAllowlist_AddIdempotent(t *testing.T) {
	a := NewProjectLabelAllowlist(3)
	a.Add("p1")
	a.Add("p1")
	if a.Size() != 1 {
		t.Errorf("dup add increased size: %d", a.Size())
	}
}

func TestProjectAllowlist_LargeWorkloadBoundedCardinality(t *testing.T) {
	a := NewProjectLabelAllowlist(51) // 50 real slots + fallback
	for i := 0; i < 500; i++ {
		a.Add("project-" + string(rune('a'+(i%26))))
	}
	if a.Size() > 50 {
		t.Errorf("size = %d, exceeded cap", a.Size())
	}
}

func TestRecordClaimLatencyByProject_NilSafe(t *testing.T) {
	var m *QueueMetrics
	// Must not panic.
	m.RecordClaimLatencyByProject(context.Background(), nil, "p1", 1.5)
}

func TestRecordClaimLatencyByProject_WithoutAllowlist(t *testing.T) {
	m, err := Metrics()
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	// No allowlist ⇒ should fall back to label-less path.
	m.RecordClaimLatencyByProject(context.Background(), nil, "p1", 0.1)
}

func TestRecordClaimLatencyByProject_WithAllowlist(t *testing.T) {
	m, err := Metrics()
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	a := NewProjectLabelAllowlist(10)
	a.Set([]string{"p1", "p2"})
	m.RecordClaimLatencyByProject(context.Background(), a, "p1", 0.1)
	m.RecordClaimLatencyByProject(context.Background(), a, "p999", 0.2) // _other
}
