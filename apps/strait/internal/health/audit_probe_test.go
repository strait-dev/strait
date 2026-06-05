package health

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, probe.Check(context.Background()))
}

func TestAuditProbe_DegradedWhenNonEmpty(t *testing.T) {
	t.Parallel()
	probe := NewAuditProbe(&fakeDLQCounter{count: 3})
	err := probe.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3 unreplayed")
}

func TestAuditProbe_ErrorPropagates(t *testing.T) {
	t.Parallel()
	probe := NewAuditProbe(&fakeDLQCounter{err: errors.New("db down")})
	err := probe.Check(context.Background())
	require.Error(t, err)
}

func TestAuditProbe_NilStoreIsHealthy(t *testing.T) {
	t.Parallel()
	probe := NewAuditProbe(nil)
	require.NoError(t, probe.Check(context.Background()))
}

func TestAuditProbe_Name(t *testing.T) {
	t.Parallel()
	probe := NewAuditProbe(nil)
	assert.Equal(t, "audit_emit_health", probe.Name())
}

func TestAuditProbe_IntegrationWithRegistry(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	counter := &fakeDLQCounter{count: 5}
	reg.Register(NewAuditProbe(counter))

	result := reg.CheckAll(context.Background())
	assert.Equal(t, StatusDegraded, result.Status)
	found := false
	for _, c := range result.Components {
		if c.Name == "audit_emit_health" {
			found = true
			assert.Equal(t, StatusDown, c.Status)
		}
	}
	assert.True(t, found)
}
