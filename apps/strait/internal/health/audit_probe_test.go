package health

import (
	"context"
	"errors"
	"testing"
)

type fakeDLQCounter struct {
	count int64
	err   error
}

func (f *fakeDLQCounter) CountAuditEventsDeadletter(_ context.Context) (int64, error) {
	return f.count, f.err
}

func TestAuditProbe_HealthyWhenEmpty(t *testing.T) {
	t.Parallel()
	probe := NewAuditProbe(&fakeDLQCounter{count: 0})
	if err := probe.Check(context.Background()); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestAuditProbe_DegradedWhenNonEmpty(t *testing.T) {
	t.Parallel()
	probe := NewAuditProbe(&fakeDLQCounter{count: 3})
	err := probe.Check(context.Background())
	if err == nil {
		t.Fatal("expected non-nil error when deadletter has rows")
	}
	if !containsSubstring(err.Error(), "3 unreplayed") {
		t.Errorf("error = %q, expected mention of 3 unreplayed rows", err.Error())
	}
}

func TestAuditProbe_ErrorPropagates(t *testing.T) {
	t.Parallel()
	probe := NewAuditProbe(&fakeDLQCounter{err: errors.New("db down")})
	err := probe.Check(context.Background())
	if err == nil {
		t.Fatal("expected error when counter errors")
	}
}

func TestAuditProbe_NilStoreIsHealthy(t *testing.T) {
	t.Parallel()
	probe := NewAuditProbe(nil)
	if err := probe.Check(context.Background()); err != nil {
		t.Errorf("expected nil store to be healthy, got %v", err)
	}
}

func TestAuditProbe_Name(t *testing.T) {
	t.Parallel()
	probe := NewAuditProbe(nil)
	if probe.Name() != "audit_emit_health" {
		t.Errorf("name = %q, want audit_emit_health", probe.Name())
	}
}

func containsSubstring(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
