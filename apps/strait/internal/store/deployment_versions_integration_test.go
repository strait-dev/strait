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
	if err := q.CreateDeploymentVersion(ctx, deployment); err != nil {
		t.Fatalf("CreateDeploymentVersion() error = %v", err)
	}

	if deployment.CreatedAt.IsZero() || deployment.UpdatedAt.IsZero() {
		t.Fatalf("CreateDeploymentVersion() timestamps not populated")
	}

	got, err := q.GetDeploymentVersion(ctx, deployment.ID, deployment.ProjectID)
	if err != nil {
		t.Fatalf("GetDeploymentVersion() error = %v", err)
	}
	if got.ID != deployment.ID {
		t.Fatalf("GetDeploymentVersion() id = %q, want %q", got.ID, deployment.ID)
	}
	if got.Status != domain.DeploymentVersionStatusDraft {
		t.Fatalf("GetDeploymentVersion() status = %q, want %q", got.Status, domain.DeploymentVersionStatusDraft)
	}
}

func TestFinalizeDeploymentVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	deployment := newDeployment("project-deploy-finalize", "staging", "bun")
	if err := q.CreateDeploymentVersion(ctx, deployment); err != nil {
		t.Fatalf("CreateDeploymentVersion() error = %v", err)
	}

	finalized, err := q.FinalizeDeploymentVersion(ctx, deployment.ID, deployment.ProjectID, "operator")
	if err != nil {
		t.Fatalf("FinalizeDeploymentVersion() error = %v", err)
	}
	if finalized.Status != domain.DeploymentVersionStatusFinalized {
		t.Fatalf("FinalizeDeploymentVersion() status = %q, want %q", finalized.Status, domain.DeploymentVersionStatusFinalized)
	}
	if finalized.FinalizedAt == nil {
		t.Fatal("FinalizeDeploymentVersion() finalized_at is nil")
	}
}

func TestPromoteDeploymentVersion_SinglePromotedPerEnvironment(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	dep1 := newDeployment("project-deploy-promote", "production", "node")
	dep2 := newDeployment("project-deploy-promote", "production", "node")

	if err := q.CreateDeploymentVersion(ctx, dep1); err != nil {
		t.Fatalf("CreateDeploymentVersion(dep1) error = %v", err)
	}
	if err := q.CreateDeploymentVersion(ctx, dep2); err != nil {
		t.Fatalf("CreateDeploymentVersion(dep2) error = %v", err)
	}
	if _, err := q.FinalizeDeploymentVersion(ctx, dep1.ID, dep1.ProjectID, "operator"); err != nil {
		t.Fatalf("FinalizeDeploymentVersion(dep1) error = %v", err)
	}
	if _, err := q.FinalizeDeploymentVersion(ctx, dep2.ID, dep2.ProjectID, "operator"); err != nil {
		t.Fatalf("FinalizeDeploymentVersion(dep2) error = %v", err)
	}

	if _, err := q.PromoteDeploymentVersion(ctx, dep1.ID, dep1.ProjectID, dep1.Environment, "operator"); err != nil {
		t.Fatalf("PromoteDeploymentVersion(dep1) error = %v", err)
	}
	promoted2, err := q.PromoteDeploymentVersion(ctx, dep2.ID, dep2.ProjectID, dep2.Environment, "operator")
	if err != nil {
		t.Fatalf("PromoteDeploymentVersion(dep2) error = %v", err)
	}
	if promoted2.Status != domain.DeploymentVersionStatusPromoted {
		t.Fatalf("PromoteDeploymentVersion(dep2) status = %q, want %q", promoted2.Status, domain.DeploymentVersionStatusPromoted)
	}

	latest, err := q.GetDeploymentVersion(ctx, dep2.ID, dep2.ProjectID)
	if err != nil {
		t.Fatalf("GetDeploymentVersion(dep2) error = %v", err)
	}
	if latest.PromotedAt == nil {
		t.Fatal("GetDeploymentVersion(dep2) promoted_at is nil")
	}

	old, err := q.GetDeploymentVersion(ctx, dep1.ID, dep1.ProjectID)
	if err != nil {
		t.Fatalf("GetDeploymentVersion(dep1) error = %v", err)
	}
	if old.PromotedAt != nil {
		t.Fatalf("GetDeploymentVersion(dep1) promoted_at = %v, want nil", old.PromotedAt)
	}
}

func TestRollbackDeploymentVersion_SetsRollbackFrom(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	dep1 := newDeployment("project-deploy-rollback", "production", "node")
	dep2 := newDeployment("project-deploy-rollback", "production", "node")

	if err := q.CreateDeploymentVersion(ctx, dep1); err != nil {
		t.Fatalf("CreateDeploymentVersion(dep1) error = %v", err)
	}
	if err := q.CreateDeploymentVersion(ctx, dep2); err != nil {
		t.Fatalf("CreateDeploymentVersion(dep2) error = %v", err)
	}
	if _, err := q.FinalizeDeploymentVersion(ctx, dep1.ID, dep1.ProjectID, "operator"); err != nil {
		t.Fatalf("FinalizeDeploymentVersion(dep1) error = %v", err)
	}
	if _, err := q.FinalizeDeploymentVersion(ctx, dep2.ID, dep2.ProjectID, "operator"); err != nil {
		t.Fatalf("FinalizeDeploymentVersion(dep2) error = %v", err)
	}
	if _, err := q.PromoteDeploymentVersion(ctx, dep2.ID, dep2.ProjectID, dep2.Environment, "operator"); err != nil {
		t.Fatalf("PromoteDeploymentVersion(dep2) error = %v", err)
	}

	rolledBack, err := q.RollbackDeploymentVersion(ctx, dep1.ID, dep1.ProjectID, dep1.Environment, "operator")
	if err != nil {
		t.Fatalf("RollbackDeploymentVersion(dep1) error = %v", err)
	}
	if rolledBack.Status != domain.DeploymentVersionStatusPromoted {
		t.Fatalf("RollbackDeploymentVersion(dep1) status = %q, want %q", rolledBack.Status, domain.DeploymentVersionStatusPromoted)
	}
	if rolledBack.RollbackFromDeployment != dep2.ID {
		t.Fatalf("RollbackDeploymentVersion(dep1) rollback_from = %q, want %q", rolledBack.RollbackFromDeployment, dep2.ID)
	}
}

func TestListDeploymentVersions(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	dep1 := newDeployment("project-deploy-list", "production", "node")
	dep2 := newDeployment("project-deploy-list", "production", "node")
	dep3 := newDeployment("project-deploy-list", "staging", "node")

	for _, dep := range []*domain.DeploymentVersion{dep1, dep2, dep3} {
		if err := q.CreateDeploymentVersion(ctx, dep); err != nil {
			t.Fatalf("CreateDeploymentVersion(%s) error = %v", dep.ID, err)
		}
		time.Sleep(time.Millisecond)
	}

	versions, err := q.ListDeploymentVersions(ctx, "project-deploy-list", "production", 10, nil)
	if err != nil {
		t.Fatalf("ListDeploymentVersions(production) error = %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("ListDeploymentVersions(production) len = %d, want 2", len(versions))
	}
	if versions[0].CreatedAt.Before(versions[1].CreatedAt) {
		t.Fatalf("ListDeploymentVersions(production) not ordered by created_at desc")
	}

	_, err = q.GetDeploymentVersion(ctx, "missing", "project-deploy-list")
	if !errors.Is(err, store.ErrDeploymentVersionNotFound) {
		t.Fatalf("GetDeploymentVersion(missing) error = %v, want ErrDeploymentVersionNotFound", err)
	}
}
