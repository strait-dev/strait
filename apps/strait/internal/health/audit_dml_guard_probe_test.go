package health

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, probe.Check(context.Background()))
	assert.True(t, checker.called)
}

func TestAuditDMLGuardProbe_DegradedWhenUnrestricted(t *testing.T) {
	t.Parallel()
	checker := &fakeAuditDMLChecker{unrestricted: true}
	probe := NewAuditDMLGuardProbe(checker)
	err := probe.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UPDATE/DELETE/TRUNCATE")
}

func TestAuditDMLGuardProbe_ErrorReturnsDegraded(t *testing.T) {
	t.Parallel()
	checker := &fakeAuditDMLChecker{err: errors.New("privilege probe failed")}
	probe := NewAuditDMLGuardProbe(checker)
	err := probe.Check(context.Background())
	require.Error(t, err)
}

func TestAuditDMLGuardProbe_NilCheckerIsHealthy(t *testing.T) {
	t.Parallel()
	probe := NewAuditDMLGuardProbe(nil)
	require.NoError(t, probe.Check(context.Background()))
}

func TestAuditDMLGuardProbe_Name(t *testing.T) {
	t.Parallel()
	probe := NewAuditDMLGuardProbe(nil)
	assert.Equal(t, "audit_dml_guard", probe.Name())
}

func TestAuditDMLGuardProbe_NonCritical(t *testing.T) {
	t.Parallel()
	// DML guard is advisory: missing restrictions degrade health but should not
	// take the service down, as self-hosted installs may never provision
	// strait_app and are still functional.
	reg := NewRegistry()
	reg.Register(NewAuditDMLGuardProbe(&fakeAuditDMLChecker{unrestricted: true}))
	result := reg.CheckAll(context.Background())
	assert.Equal(t, StatusDegraded, result.Status)
}
