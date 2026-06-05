//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func newDeployment(projectID, environment, runtime string) *domain.DeploymentVersion {
	return &domain.DeploymentVersion{
		ID:          newID(),
		ProjectID:   projectID,
		Environment: environment,
		Runtime:     runtime,
		ArtifactURI: "https://example.com/artifacts/manifest.tgz",
		Manifest:    json.RawMessage(`{"jobs":2}`),
		Checksum:    "sha256:test",
		Status:      domain.DeploymentVersionStatusDraft,
		CreatedBy:   "tester",
		UpdatedBy:   "tester",
	}
}

func TestCreateAndGetDeploymentVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	deployment := newDeployment("project-deploy-create", "production", "node")
	require.NoError(t, q.CreateDeploymentVersion(ctx, deployment))
	require.False(t, deployment.
		CreatedAt.
		IsZero() || deployment.
		UpdatedAt.
		IsZero())

	got, err := q.GetDeploymentVersion(ctx, deployment.ID, deployment.ProjectID)
	require.NoError(t, err)
	require.Equal(t, deployment.
		ID, got.
		ID)
	require.Equal(t, domain.
		DeploymentVersionStatusDraft,

		got.Status,
	)

}

func TestFinalizeDeploymentVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	deployment := newDeployment("project-deploy-finalize", "staging", "bun")
	require.NoError(t, q.CreateDeploymentVersion(ctx, deployment))

	finalized, err := q.FinalizeDeploymentVersion(ctx, deployment.ID, deployment.ProjectID, "operator")
	require.NoError(t, err)
	require.Equal(t, domain.
		DeploymentVersionStatusFinalized,

		finalized.
			Status,
	)
	require.NotNil(t, finalized.
		FinalizedAt,
	)

}

func TestPromoteDeploymentVersion_SinglePromotedPerEnvironment(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	dep1 := newDeployment("project-deploy-promote", "production", "node")
	dep2 := newDeployment("project-deploy-promote", "production", "node")
	require.NoError(t, q.CreateDeploymentVersion(ctx, dep1))
	require.NoError(t, q.CreateDeploymentVersion(ctx, dep2))

	if _, err := q.FinalizeDeploymentVersion(ctx, dep1.ID, dep1.ProjectID, "operator"); err != nil {
		require.Failf(t, "test failure",

			"FinalizeDeploymentVersion(dep1) error = %v", err)
	}
	if _, err := q.FinalizeDeploymentVersion(ctx, dep2.ID, dep2.ProjectID, "operator"); err != nil {
		require.Failf(t, "test failure",

			"FinalizeDeploymentVersion(dep2) error = %v", err)
	}

	if _, err := q.PromoteDeploymentVersion(ctx, dep1.ID, dep1.ProjectID, dep1.Environment, "operator"); err != nil {
		require.Failf(t, "test failure",

			"PromoteDeploymentVersion(dep1) error = %v", err)
	}
	promoted2, err := q.PromoteDeploymentVersion(ctx, dep2.ID, dep2.ProjectID, dep2.Environment, "operator")
	require.NoError(t, err)
	require.Equal(t, domain.
		DeploymentVersionStatusPromoted,

		promoted2.
			Status,
	)

	latest, err := q.GetDeploymentVersion(ctx, dep2.ID, dep2.ProjectID)
	require.NoError(t, err)
	require.NotNil(t, latest.
		PromotedAt,
	)

	old, err := q.GetDeploymentVersion(ctx, dep1.ID, dep1.ProjectID)
	require.NoError(t, err)
	require.Nil(t, old.
		PromotedAt,
	)

}

func TestRollbackDeploymentVersion_SetsRollbackFrom(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	dep1 := newDeployment("project-deploy-rollback", "production", "node")
	dep2 := newDeployment("project-deploy-rollback", "production", "node")
	require.NoError(t, q.CreateDeploymentVersion(ctx, dep1))
	require.NoError(t, q.CreateDeploymentVersion(ctx, dep2))

	if _, err := q.FinalizeDeploymentVersion(ctx, dep1.ID, dep1.ProjectID, "operator"); err != nil {
		require.Failf(t, "test failure",

			"FinalizeDeploymentVersion(dep1) error = %v", err)
	}
	if _, err := q.FinalizeDeploymentVersion(ctx, dep2.ID, dep2.ProjectID, "operator"); err != nil {
		require.Failf(t, "test failure",

			"FinalizeDeploymentVersion(dep2) error = %v", err)
	}
	if _, err := q.PromoteDeploymentVersion(ctx, dep2.ID, dep2.ProjectID, dep2.Environment, "operator"); err != nil {
		require.Failf(t, "test failure",

			"PromoteDeploymentVersion(dep2) error = %v", err)
	}

	rolledBack, err := q.RollbackDeploymentVersion(ctx, dep1.ID, dep1.ProjectID, dep1.Environment, "operator")
	require.NoError(t, err)
	require.Equal(t, domain.
		DeploymentVersionStatusPromoted,

		rolledBack.
			Status,
	)
	require.Equal(t, dep2.ID,

		rolledBack.
			RollbackFromDeployment,
	)

}

func TestListDeploymentVersions(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	dep1 := newDeployment("project-deploy-list", "production", "node")
	dep2 := newDeployment("project-deploy-list", "production", "node")
	dep3 := newDeployment("project-deploy-list", "staging", "node")

	for _, dep := range []*domain.DeploymentVersion{dep1, dep2, dep3} {
		require.NoError(t, q.CreateDeploymentVersion(ctx, dep))

		time.Sleep(time.Millisecond)
	}

	versions, err := q.ListDeploymentVersions(ctx, "project-deploy-list", "production", 10, nil)
	require.NoError(t, err)
	require.Len(t, versions,

		2)
	require.False(t, versions[0].CreatedAt.
		Before(versions[1].CreatedAt))

	_, err = q.GetDeploymentVersion(ctx, "missing", "project-deploy-list")
	require.True(t, errors.Is(err, store.
		ErrDeploymentVersionNotFound,
	))

}
