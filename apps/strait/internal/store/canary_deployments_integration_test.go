//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestCreateCanaryDeployment(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-canary-create"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})

	canary := &domain.CanaryDeployment{
		WorkflowID:    wf.ID,
		ProjectID:     projectID,
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    10,
		Status:        "active",
		AutoPromote:   json.RawMessage(`{"enabled":true}`),
	}
	require.NoError(t, q.CreateCanaryDeployment(
		ctx, canary))
	require.NotEqual(t, "",

		canary.ID,
	)
	require.False(t, canary.
		CreatedAt.
		IsZero())

}

func TestCreateCanaryDeployment_DuplicateActive(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-canary-dup"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})

	canary := &domain.CanaryDeployment{
		WorkflowID:    wf.ID,
		ProjectID:     projectID,
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    10,
		Status:        "active",
	}
	require.NoError(t, q.CreateCanaryDeployment(
		ctx, canary))

	dup := &domain.CanaryDeployment{
		WorkflowID:    wf.ID,
		ProjectID:     projectID,
		SourceVersion: 1,
		TargetVersion: 3,
		TrafficPct:    5,
		Status:        "active",
	}
	err := q.CreateCanaryDeployment(ctx, dup)
	require.True(t, errors.Is(err, store.
		ErrCanaryAlreadyActive,
	))

}

func TestGetActiveCanaryDeployment(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-canary-get"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})

	canary := &domain.CanaryDeployment{
		WorkflowID:    wf.ID,
		ProjectID:     projectID,
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    15,
		Status:        "active",
	}
	require.NoError(t, q.CreateCanaryDeployment(
		ctx, canary))

	got, err := q.GetActiveCanaryDeployment(ctx, wf.ID)
	require.NoError(t, err)
	require.Equal(t, canary.
		ID, got.ID,
	)
	require.EqualValues(t, 15, got.
		TrafficPct,
	)

	// Not found.
	_, err = q.GetActiveCanaryDeployment(ctx, newID())
	require.True(t, errors.Is(err, store.
		ErrCanaryNotFound,
	))

}

func TestUpdateCanaryDeploymentTraffic(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-canary-traffic"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})

	canary := &domain.CanaryDeployment{
		WorkflowID:    wf.ID,
		ProjectID:     projectID,
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    10,
		Status:        "active",
	}
	require.NoError(t, q.CreateCanaryDeployment(
		ctx, canary))
	require.NoError(t, q.UpdateCanaryDeploymentTraffic(ctx, wf.ID,
		50))

	got, err := q.GetActiveCanaryDeployment(ctx, wf.ID)
	require.NoError(t, err)
	require.EqualValues(t, 50, got.
		TrafficPct,
	)

	// Not found.
	err = q.UpdateCanaryDeploymentTraffic(ctx, newID(), 50)
	require.True(t, errors.Is(err, store.
		ErrCanaryNotFound,
	))

}

func TestCompleteCanaryDeployment(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-canary-complete"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})

	canary := &domain.CanaryDeployment{
		WorkflowID:    wf.ID,
		ProjectID:     projectID,
		SourceVersion: 1,
		TargetVersion: 2,
		TrafficPct:    10,
		Status:        "active",
	}
	require.NoError(t, q.CreateCanaryDeployment(
		ctx, canary))
	require.NoError(t, q.CompleteCanaryDeployment(ctx, wf.ID, "completed"))

	// Should now be not found (no longer active).
	_, err := q.GetActiveCanaryDeployment(ctx, wf.ID)
	require.True(t, errors.Is(err, store.
		ErrCanaryNotFound,
	))

	// Not found.
	err = q.CompleteCanaryDeployment(ctx, newID(), "completed")
	require.True(t, errors.Is(err, store.
		ErrCanaryNotFound,
	))

}
