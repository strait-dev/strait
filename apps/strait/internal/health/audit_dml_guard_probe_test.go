package health

import (
	"context"
	"errors"
	"testing"
)

type fakeAuditDMLChecker struct {
	unrestricted bool
	err          error
	called       bool
}

func (f *fakeAuditDMLChecker) AuditEventsDMLRestricted(_ context.Context) (bool, error) {
	f.called = true
	return !f.unrestricted, f.err
}

func TestAuditDMLGuardProbe_EnforcedWhenRestricted(t *testing.T) {
	t.Parallel()
	checker := &fakeAuditDMLChecker{unrestricted: false}
	probe := NewAuditDMLGuardProbe(checker)
	if err := probe.Check(context.Background()); err != nil {
		t.Errorf("expected nil error when UPDATE is restricted, got %v", err)
	}
	if !checker.called {
		t.Error("expected checker to be invoked")
	}
}

func TestAuditDMLGuardProbe_DegradedWhenUnrestricted(t *testing.T) {
	t.Parallel()
	checker := &fakeAuditDMLChecker{unrestricted: true}
	probe := NewAuditDMLGuardProbe(checker)
	err := probe.Check(context.Background())
	if err == nil {
		t.Fatal("expected non-nil error when UPDATE is unrestricted")
	}
	if !containsSubstring(err.Error(), "UPDATE/DELETE/TRUNCATE") {
		t.Errorf("error = %q, expected mention of UPDATE/DELETE/TRUNCATE", err.Error())
	}
}

func TestAuditDMLGuardProbe_ErrorReturnsDegraded(t *testing.T) {
	t.Parallel()
	checker := &fakeAuditDMLChecker{err: errors.New("privilege probe failed")}
	probe := NewAuditDMLGuardProbe(checker)
	err := probe.Check(context.Background())
	if err == nil {
		t.Fatal("expected error when privilege probe errors")
	}
}

func TestAuditDMLGuardProbe_NilCheckerIsHealthy(t *testing.T) {
	t.Parallel()
	probe := NewAuditDMLGuardProbe(nil)
	if err := probe.Check(context.Background()); err != nil {
		t.Errorf("expected nil checker to be healthy, got %v", err)
	}
}

func TestAuditDMLGuardProbe_Name(t *testing.T) {
	t.Parallel()
	probe := NewAuditDMLGuardProbe(nil)
	if probe.Name() != "audit_dml_guard" {
		t.Errorf("name = %q, want audit_dml_guard", probe.Name())
	}
}

func TestAuditDMLGuardProbe_NonCritical(t *testing.T) {
	t.Parallel()
	// DML guard is advisory: missing restrictions degrade health but should not
	// take the service down, as self-hosted installs may never provision
	// strait_app and are still functional.
	reg := NewRegistry()
	reg.Register(NewAuditDMLGuardProbe(&fakeAuditDMLChecker{unrestricted: true}))
	result := reg.CheckAll(context.Background())
	if result.Status != StatusDegraded {
		t.Errorf("status = %q, want degraded", result.Status)
	}
}
