//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestCreateLogDrain(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	drain := &domain.LogDrain{
		ID:          newID(),
		ProjectID:   "project-log-drain-create",
		Name:        "drain-1",
		DrainType:   "http",
		EndpointURL: "https://logs.example.com/ingest",
		AuthType:    "bearer",
		AuthConfig:  map[string]string{"token": "secret-token"},
		LevelFilter: []string{"error", "warn"},
		Enabled:     true,
	}
	require.NoError(t, st.CreateLogDrain(ctx, drain))

	got, err := st.GetLogDrain(ctx, drain.ID, drain.ProjectID)
	require.NoError(t, err)
	require.Equal(t, drain.
		ID,
		got.ID,
	)
	require.Equal(t, drain.
		ProjectID,

		got.ProjectID,
	)
	require.Equal(t, drain.
		Name,
		got.
			Name)
	require.Equal(t, drain.
		DrainType,

		got.DrainType,
	)
	require.Equal(t, drain.
		EndpointURL,

		got.EndpointURL,
	)
	require.Equal(t, drain.
		AuthType,

		got.AuthType,
	)
	require.Equal(t, "secret-token",

		got.AuthConfig["token"])
	require.Len(t, got.LevelFilter,

		2,
	)
	require.False(t, got.LevelFilter[0] != "error" ||
		got.
			LevelFilter[1] != "warn",
	)
	require.True(t, got.Enabled)
	require.False(t, got.CreatedAt.
		IsZero())
	require.False(t, got.UpdatedAt.
		IsZero())

}

func TestGetLogDrain_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	_, err := st.GetLogDrain(ctx, newID(), "project-nonexistent")
	require.True(t, errors.Is(err, store.
		ErrLogDrainNotFound,
	))

}

func TestListLogDrains(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	projectID := "project-log-drain-list"

	d1 := &domain.LogDrain{
		ID: newID(), ProjectID: projectID, Name: "drain-1",
		DrainType: "http", EndpointURL: "https://example.com/1",
		AuthType: "none", LevelFilter: []string{}, Enabled: true,
	}
	require.NoError(t, st.CreateLogDrain(ctx, d1))

	time.Sleep(time.Millisecond)

	d2 := &domain.LogDrain{
		ID: newID(), ProjectID: projectID, Name: "drain-2",
		DrainType: "http", EndpointURL: "https://example.com/2",
		AuthType: "none", LevelFilter: []string{}, Enabled: true,
	}
	require.NoError(t, st.CreateLogDrain(ctx, d2))

	drains, err := st.ListLogDrains(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, drains,
		2,
	)
	require.Equal(t, d2.ID,

		drains[0].
			ID)
	require.Equal(t, d1.ID,

		drains[1].
			ID)

	// Ordered by created_at DESC: d2 first

}

func TestListLogDrains_CrossProject(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	drainA := &domain.LogDrain{
		ID: newID(), ProjectID: "project-a", Name: "drain-a",
		DrainType: "http", EndpointURL: "https://example.com/a",
		AuthType: "none", LevelFilter: []string{}, Enabled: true,
	}
	require.NoError(t, st.CreateLogDrain(ctx, drainA))

	drainB := &domain.LogDrain{
		ID: newID(), ProjectID: "project-b", Name: "drain-b",
		DrainType: "http", EndpointURL: "https://example.com/b",
		AuthType: "none", LevelFilter: []string{}, Enabled: true,
	}
	require.NoError(t, st.CreateLogDrain(ctx, drainB))

	drains, err := st.ListLogDrains(ctx, "project-a")
	require.NoError(t, err)
	require.Len(t, drains,
		1,
	)
	require.Equal(t, drainA.
		ID, drains[0].ID)

}

func TestUpdateLogDrain(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	drain := &domain.LogDrain{
		ID: newID(), ProjectID: "project-log-drain-update", Name: "original-name",
		DrainType: "http", EndpointURL: "https://example.com/update",
		AuthType: "none", LevelFilter: []string{}, Enabled: true,
	}
	require.NoError(t, st.CreateLogDrain(ctx, drain))

	original, err := st.GetLogDrain(ctx, drain.ID, drain.ProjectID)
	require.NoError(t, err)

	time.Sleep(time.Millisecond)

	patch := map[string]any{"name": "updated-drain", "enabled": false}
	require.NoError(t, st.UpdateLogDrain(ctx, drain.
		ID, drain.
		ProjectID,
		patch,
	))

	got, err := st.GetLogDrain(ctx, drain.ID, drain.ProjectID)
	require.NoError(t, err)
	require.Equal(t, "updated-drain",

		got.Name)
	require.False(t, got.Enabled)
	require.True(t, got.UpdatedAt.
		After(original.
			UpdatedAt,
		))

}

func TestDeleteLogDrain(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	drain := &domain.LogDrain{
		ID: newID(), ProjectID: "project-log-drain-delete", Name: "to-delete",
		DrainType: "http", EndpointURL: "https://example.com/delete",
		AuthType: "none", LevelFilter: []string{}, Enabled: true,
	}
	require.NoError(t, st.CreateLogDrain(ctx, drain))
	require.NoError(t, st.DeleteLogDrain(ctx, drain.
		ID, drain.
		ProjectID,
	))

	_, err := st.GetLogDrain(ctx, drain.ID, drain.ProjectID)
	require.True(t, errors.Is(err, store.
		ErrLogDrainNotFound,
	))

	err = st.DeleteLogDrain(ctx, drain.ID, drain.ProjectID)
	require.True(t, errors.Is(err, store.
		ErrLogDrainNotFound,
	))

}
