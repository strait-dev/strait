package cache

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestStatusReadModel_CASRejectsOutOfOrderUpdate(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	model := NewReadModel[string](ReadModelConfig[string]{
		Client:    rdb,
		Namespace: "status_test",
		TTL:       time.Minute,
	})

	ok, err := model.CompareAndSet(t.Context(), "run-1", "running", 5)
	require.NoError(t, err)
	require.True(t,
		ok)

	ok, err = model.CompareAndSet(t.Context(), "run-1", "queued", 4)
	require.NoError(t, err)
	require.False(t,
		ok)

	got, err := model.Get(t.Context(), "run-1")
	require.NoError(t, err)
	require.False(t,
		got.Version !=
			5 || got.Value != "running")

}

func TestStatusReadModel_SetIfColdDoesNotOverwriteNewerCDCValue(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	model := NewReadModel[string](ReadModelConfig[string]{
		Client:    rdb,
		Namespace: "status_fill_test",
		TTL:       time.Minute,
	})

	ok, err := model.CompareAndSet(t.Context(), "run-1", "completed", 9)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, model.SetIfCold(t.Context(), "run-1", "queued"))

	got, err := model.Get(t.Context(), "run-1")
	require.NoError(t, err)
	require.False(t,
		got.Version !=
			9 || got.Value != "completed",
	)

}

func TestStatusReadModel_SetIfColdVersionRejectsOlderCDCOverwrite(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	model := NewReadModel[string](ReadModelConfig[string]{
		Client:    rdb,
		Namespace: "status_fill_version_test",
		TTL:       time.Minute,
	})
	require.NoError(t, model.
		SetIfColdVersion(t.Context(), "run-1",
			"executing", 10))

	ok, err := model.CompareAndSet(t.Context(), "run-1", "queued", 7)
	require.NoError(t, err)
	require.False(t,
		ok)

	got, err := model.Get(t.Context(), "run-1")
	require.NoError(t, err)
	require.False(t,
		got.Version !=
			10 || got.Value != "executing",
	)

}
