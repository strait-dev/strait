package health

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsCritical_DefaultIsTrue(t *testing.T) {
	t.Parallel()
	c := NewChecker("plain", func(context.Context) error { return nil })
	assert.True(t, IsCritical(c))
}

func TestIsCritical_CriticalCheckerTrue(t *testing.T) {
	t.Parallel()
	c := NewCriticalChecker("db", true, func(context.Context) error { return nil })
	assert.True(t, IsCritical(c))
}

func TestIsCritical_CriticalCheckerFalse(t *testing.T) {
	t.Parallel()
	c := NewCriticalChecker("redis", false, func(context.Context) error { return nil })
	assert.False(t, IsCritical(c))
}

func TestRegistry_NonCriticalDown_ReportsDegraded(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewChecker("db", func(context.Context) error { return nil }))
	r.Register(NewCriticalChecker("redis", false, func(context.Context) error {
		return errors.New("connection refused")
	}))

	result := r.CheckAll(context.Background())
	require.Equal(t, StatusDegraded, result.Status)
}

func TestRegistry_CriticalDown_ReportsDown(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewCriticalChecker("db", true, func(context.Context) error {
		return errors.New("database unreachable")
	}))
	r.Register(NewCriticalChecker("redis", false, func(context.Context) error { return nil }))

	result := r.CheckAll(context.Background())
	require.Equal(t, StatusDown, result.Status)
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
	require.Equal(t, StatusDown, result.Status)
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
	require.Equal(t, StatusDegraded, result.Status)

	downCount := 0
	for _, c := range result.Components {
		if c.Status == StatusDown {
			downCount++
		}
	}
	require.Equal(t, 2, downCount)
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
	require.Equal(t, StatusDegraded, result.Status)
}

func TestCriticalChecker_DelegatesCheck(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("check failed")
	c := NewCriticalChecker("test", false, func(context.Context) error {
		return wantErr
	})

	assert.Equal(t, "test", c.Name())
	require.ErrorIs(t, c.Check(context.Background()), wantErr)
}

func TestRegistry_DegradedComponentErrorsVisible(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(NewChecker("db", func(context.Context) error { return nil }))
	r.Register(NewCriticalChecker("redis", false, func(context.Context) error {
		return errors.New("dial tcp: connection refused")
	}))

	result := r.CheckAll(context.Background())
	require.Equal(t, StatusDegraded, result.Status)

	for _, c := range result.Components {
		if c.Name == "redis" {
			require.Equal(t, "dial tcp: connection refused", c.Error)
			return
		}
	}
	require.Fail(t, "redis component not found in results")
}
