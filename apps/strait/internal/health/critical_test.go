package health

import (
	"context"
	"errors"
	"testing"
)

func TestIsCritical_DefaultIsTrue(t *testing.T) {
	t.Parallel()
	c := NewChecker("plain", func(context.Context) error { return nil })
	if !IsCritical(c) {
		t.Fatal("plain Checker should be critical by default")
	}
}

func TestIsCritical_CriticalCheckerTrue(t *testing.T) {
	t.Parallel()
	c := NewCriticalChecker("db", true, func(context.Context) error { return nil })
	if !IsCritical(c) {
		t.Fatal("CriticalChecker(critical=true) should be critical")
	}
}

func TestIsCritical_CriticalCheckerFalse(t *testing.T) {
	t.Parallel()
	c := NewCriticalChecker("redis", false, func(context.Context) error { return nil })
	if IsCritical(c) {
		t.Fatal("CriticalChecker(critical=false) should not be critical")
	}
}

func TestRegistry_NonCriticalDown_ReportsDegraded(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewChecker("db", func(context.Context) error { return nil }))
	r.Register(NewCriticalChecker("redis", false, func(context.Context) error {
		return errors.New("connection refused")
	}))

	result := r.CheckAll(context.Background())
	if result.Status != StatusDegraded {
		t.Fatalf("status = %q, want %q", result.Status, StatusDegraded)
	}
}

func TestRegistry_CriticalDown_ReportsDown(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewCriticalChecker("db", true, func(context.Context) error {
		return errors.New("database unreachable")
	}))
	r.Register(NewCriticalChecker("redis", false, func(context.Context) error { return nil }))

	result := r.CheckAll(context.Background())
	if result.Status != StatusDown {
		t.Fatalf("status = %q, want %q", result.Status, StatusDown)
	}
}

func TestRegistry_BothCriticalAndNonCriticalDown_ReportsDown(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewCriticalChecker("db", true, func(context.Context) error {
		return errors.New("database unreachable")
	}))
	r.Register(NewCriticalChecker("redis", false, func(context.Context) error {
		return errors.New("redis unreachable")
	}))

	result := r.CheckAll(context.Background())
	if result.Status != StatusDown {
		t.Fatalf("status = %q, want %q when critical is down", result.Status, StatusDown)
	}
}

func TestRegistry_MultipleNonCriticalDown_ReportsDegraded(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewChecker("db", func(context.Context) error { return nil }))
	r.Register(NewCriticalChecker("redis", false, func(context.Context) error {
		return errors.New("redis down")
	}))
	r.Register(NewCriticalChecker("sequin", false, func(context.Context) error {
		return errors.New("sequin down")
	}))

	result := r.CheckAll(context.Background())
	if result.Status != StatusDegraded {
		t.Fatalf("status = %q, want %q when only non-critical are down", result.Status, StatusDegraded)
	}

	downCount := 0
	for _, c := range result.Components {
		if c.Status == StatusDown {
			downCount++
		}
	}
	if downCount != 2 {
		t.Fatalf("down components = %d, want 2", downCount)
	}
}

func TestRegistry_AllNonCriticalDown_ReportsDegraded(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewCriticalChecker("redis", false, func(context.Context) error {
		return errors.New("redis down")
	}))
	r.Register(NewCriticalChecker("sequin", false, func(context.Context) error {
		return errors.New("sequin down")
	}))

	result := r.CheckAll(context.Background())
	if result.Status != StatusDegraded {
		t.Fatalf("status = %q, want %q when all checkers are non-critical", result.Status, StatusDegraded)
	}
}

func TestCriticalChecker_DelegatesCheck(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("check failed")
	c := NewCriticalChecker("test", false, func(context.Context) error {
		return wantErr
	})

	if c.Name() != "test" {
		t.Fatalf("name = %q, want %q", c.Name(), "test")
	}
	if err := c.Check(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestRegistry_DegradedComponentErrorsVisible(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewChecker("db", func(context.Context) error { return nil }))
	r.Register(NewCriticalChecker("redis", false, func(context.Context) error {
		return errors.New("dial tcp: connection refused")
	}))

	result := r.CheckAll(context.Background())
	if result.Status != StatusDegraded {
		t.Fatalf("status = %q, want degraded", result.Status)
	}

	for _, c := range result.Components {
		if c.Name == "redis" {
			if c.Error != "dial tcp: connection refused" {
				t.Fatalf("redis error = %q, want specific error message", c.Error)
			}
			return
		}
	}
	t.Fatal("redis component not found in results")
}
