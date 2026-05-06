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
	if err := q.CreateCanaryDeployment(ctx, canary); err != nil {
		t.Fatalf("CreateCanaryDeployment() error = %v", err)
	}
	if canary.ID == "" {
		t.Fatal("CreateCanaryDeployment() did not set ID")
	}
	if canary.CreatedAt.IsZero() {
		t.Fatal("CreateCanaryDeployment() did not set CreatedAt")
	}
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
	if err := q.CreateCanaryDeployment(ctx, canary); err != nil {
		t.Fatalf("CreateCanaryDeployment() error = %v", err)
	}

	dup := &domain.CanaryDeployment{
		WorkflowID:    wf.ID,
		ProjectID:     projectID,
		SourceVersion: 1,
		TargetVersion: 3,
		TrafficPct:    5,
		Status:        "active",
	}
	err := q.CreateCanaryDeployment(ctx, dup)
	if !errors.Is(err, store.ErrCanaryAlreadyActive) {
		t.Fatalf("CreateCanaryDeployment(dup) error = %v, want ErrCanaryAlreadyActive", err)
	}
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
	if err := q.CreateCanaryDeployment(ctx, canary); err != nil {
		t.Fatalf("CreateCanaryDeployment() error = %v", err)
	}

	got, err := q.GetActiveCanaryDeployment(ctx, wf.ID)
	if err != nil {
		t.Fatalf("GetActiveCanaryDeployment() error = %v", err)
	}
	if got.ID != canary.ID {
		t.Fatalf("ID = %q, want %q", got.ID, canary.ID)
	}
	if got.TrafficPct != 15 {
		t.Fatalf("TrafficPct = %d, want 15", got.TrafficPct)
	}

	// Not found.
	_, err = q.GetActiveCanaryDeployment(ctx, newID())
	if !errors.Is(err, store.ErrCanaryNotFound) {
		t.Fatalf("GetActiveCanaryDeployment(notfound) error = %v, want ErrCanaryNotFound", err)
	}
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
	if err := q.CreateCanaryDeployment(ctx, canary); err != nil {
		t.Fatalf("CreateCanaryDeployment() error = %v", err)
	}

	if err := q.UpdateCanaryDeploymentTraffic(ctx, wf.ID, 50); err != nil {
		t.Fatalf("UpdateCanaryDeploymentTraffic() error = %v", err)
	}

	got, err := q.GetActiveCanaryDeployment(ctx, wf.ID)
	if err != nil {
		t.Fatalf("GetActiveCanaryDeployment() error = %v", err)
	}
	if got.TrafficPct != 50 {
		t.Fatalf("TrafficPct = %d, want 50", got.TrafficPct)
	}

	// Not found.
	err = q.UpdateCanaryDeploymentTraffic(ctx, newID(), 50)
	if !errors.Is(err, store.ErrCanaryNotFound) {
		t.Fatalf("UpdateCanaryDeploymentTraffic(notfound) error = %v, want ErrCanaryNotFound", err)
	}
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
	if err := q.CreateCanaryDeployment(ctx, canary); err != nil {
		t.Fatalf("CreateCanaryDeployment() error = %v", err)
	}

	if err := q.CompleteCanaryDeployment(ctx, wf.ID, "completed"); err != nil {
		t.Fatalf("CompleteCanaryDeployment() error = %v", err)
	}

	// Should now be not found (no longer active).
	_, err := q.GetActiveCanaryDeployment(ctx, wf.ID)
	if !errors.Is(err, store.ErrCanaryNotFound) {
		t.Fatalf("GetActiveCanaryDeployment(after complete) error = %v, want ErrCanaryNotFound", err)
	}

	// Not found.
	err = q.CompleteCanaryDeployment(ctx, newID(), "completed")
	if !errors.Is(err, store.ErrCanaryNotFound) {
		t.Fatalf("CompleteCanaryDeployment(notfound) error = %v, want ErrCanaryNotFound", err)
	}
}
