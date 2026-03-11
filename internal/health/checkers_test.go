package health

import (
	"context"
	"errors"
	"testing"
)

type mockPool struct {
	available int
	active    int
}

func (m *mockPool) Available() int   { return m.available }
func (m *mockPool) ActiveCount() int { return m.active }

func TestPoolChecker_Healthy(t *testing.T) {
	t.Parallel()
	checker := NewPoolChecker(&mockPool{available: 5, active: 3})
	if err := checker.Check(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestPoolChecker_Exhausted(t *testing.T) {
	t.Parallel()
	checker := NewPoolChecker(&mockPool{available: 0, active: 10})
	if err := checker.Check(context.Background()); err == nil {
		t.Error("expected error for exhausted pool")
	}
}

func TestPoolChecker_IdlePool(t *testing.T) {
	t.Parallel()
	checker := NewPoolChecker(&mockPool{available: 0, active: 0})
	if err := checker.Check(context.Background()); err != nil {
		t.Errorf("idle pool should be healthy, got %v", err)
	}
}

func TestMigrationChecker_Clean(t *testing.T) {
	t.Parallel()
	checker := NewMigrationChecker(42, false, nil)
	if err := checker.Check(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestMigrationChecker_Dirty(t *testing.T) {
	t.Parallel()
	checker := NewMigrationChecker(42, true, nil)
	if err := checker.Check(context.Background()); err == nil {
		t.Error("expected error for dirty migration")
	}
}

func TestMigrationChecker_Error(t *testing.T) {
	t.Parallel()
	checker := NewMigrationChecker(0, false, errors.New("connection refused"))
	if err := checker.Check(context.Background()); err == nil {
		t.Error("expected error for migration check failure")
	}
}
