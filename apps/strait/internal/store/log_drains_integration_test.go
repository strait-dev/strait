//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
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

	if err := st.CreateLogDrain(ctx, drain); err != nil {
		t.Fatalf("CreateLogDrain() error = %v", err)
	}

	got, err := st.GetLogDrain(ctx, drain.ID, drain.ProjectID)
	if err != nil {
		t.Fatalf("GetLogDrain() error = %v", err)
	}

	if got.ID != drain.ID {
		t.Fatalf("ID = %q, want %q", got.ID, drain.ID)
	}
	if got.ProjectID != drain.ProjectID {
		t.Fatalf("ProjectID = %q, want %q", got.ProjectID, drain.ProjectID)
	}
	if got.Name != drain.Name {
		t.Fatalf("Name = %q, want %q", got.Name, drain.Name)
	}
	if got.DrainType != drain.DrainType {
		t.Fatalf("DrainType = %q, want %q", got.DrainType, drain.DrainType)
	}
	if got.EndpointURL != drain.EndpointURL {
		t.Fatalf("EndpointURL = %q, want %q", got.EndpointURL, drain.EndpointURL)
	}
	if got.AuthType != drain.AuthType {
		t.Fatalf("AuthType = %q, want %q", got.AuthType, drain.AuthType)
	}
	if got.AuthConfig["token"] != "secret-token" {
		t.Fatalf("AuthConfig[token] = %q, want %q", got.AuthConfig["token"], "secret-token")
	}
	if len(got.LevelFilter) != 2 {
		t.Fatalf("LevelFilter len = %d, want 2", len(got.LevelFilter))
	}
	if got.LevelFilter[0] != "error" || got.LevelFilter[1] != "warn" {
		t.Fatalf("LevelFilter = %v, want [error warn]", got.LevelFilter)
	}
	if !got.Enabled {
		t.Fatalf("Enabled = false, want true")
	}
	if got.CreatedAt.IsZero() {
		t.Fatalf("CreatedAt is zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Fatalf("UpdatedAt is zero")
	}
}

func TestGetLogDrain_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	_, err := st.GetLogDrain(ctx, newID(), "project-nonexistent")
	if !errors.Is(err, store.ErrLogDrainNotFound) {
		t.Fatalf("GetLogDrain() error = %v, want ErrLogDrainNotFound", err)
	}
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
	if err := st.CreateLogDrain(ctx, d1); err != nil {
		t.Fatalf("CreateLogDrain(d1) error = %v", err)
	}

	time.Sleep(time.Millisecond)

	d2 := &domain.LogDrain{
		ID: newID(), ProjectID: projectID, Name: "drain-2",
		DrainType: "http", EndpointURL: "https://example.com/2",
		AuthType: "none", LevelFilter: []string{}, Enabled: true,
	}
	if err := st.CreateLogDrain(ctx, d2); err != nil {
		t.Fatalf("CreateLogDrain(d2) error = %v", err)
	}

	drains, err := st.ListLogDrains(ctx, projectID)
	if err != nil {
		t.Fatalf("ListLogDrains() error = %v", err)
	}
	if len(drains) != 2 {
		t.Fatalf("ListLogDrains() len = %d, want 2", len(drains))
	}
	// Ordered by created_at DESC: d2 first
	if drains[0].ID != d2.ID {
		t.Fatalf("drains[0].ID = %q, want %q (most recent)", drains[0].ID, d2.ID)
	}
	if drains[1].ID != d1.ID {
		t.Fatalf("drains[1].ID = %q, want %q (oldest)", drains[1].ID, d1.ID)
	}
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
	if err := st.CreateLogDrain(ctx, drainA); err != nil {
		t.Fatalf("CreateLogDrain(a) error = %v", err)
	}

	drainB := &domain.LogDrain{
		ID: newID(), ProjectID: "project-b", Name: "drain-b",
		DrainType: "http", EndpointURL: "https://example.com/b",
		AuthType: "none", LevelFilter: []string{}, Enabled: true,
	}
	if err := st.CreateLogDrain(ctx, drainB); err != nil {
		t.Fatalf("CreateLogDrain(b) error = %v", err)
	}

	drains, err := st.ListLogDrains(ctx, "project-a")
	if err != nil {
		t.Fatalf("ListLogDrains(project-a) error = %v", err)
	}
	if len(drains) != 1 {
		t.Fatalf("ListLogDrains(project-a) len = %d, want 1", len(drains))
	}
	if drains[0].ID != drainA.ID {
		t.Fatalf("drains[0].ID = %q, want %q", drains[0].ID, drainA.ID)
	}
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
	if err := st.CreateLogDrain(ctx, drain); err != nil {
		t.Fatalf("CreateLogDrain() error = %v", err)
	}

	original, err := st.GetLogDrain(ctx, drain.ID, drain.ProjectID)
	if err != nil {
		t.Fatalf("GetLogDrain() before update error = %v", err)
	}

	time.Sleep(time.Millisecond)

	patch := map[string]any{"name": "updated-drain", "enabled": false}
	if err := st.UpdateLogDrain(ctx, drain.ID, drain.ProjectID, patch); err != nil {
		t.Fatalf("UpdateLogDrain() error = %v", err)
	}

	got, err := st.GetLogDrain(ctx, drain.ID, drain.ProjectID)
	if err != nil {
		t.Fatalf("GetLogDrain() after update error = %v", err)
	}
	if got.Name != "updated-drain" {
		t.Fatalf("Name = %q, want %q", got.Name, "updated-drain")
	}
	if got.Enabled {
		t.Fatalf("Enabled = true, want false")
	}
	if !got.UpdatedAt.After(original.UpdatedAt) {
		t.Fatalf("UpdatedAt %v not after original %v", got.UpdatedAt, original.UpdatedAt)
	}
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
	if err := st.CreateLogDrain(ctx, drain); err != nil {
		t.Fatalf("CreateLogDrain() error = %v", err)
	}

	if err := st.DeleteLogDrain(ctx, drain.ID, drain.ProjectID); err != nil {
		t.Fatalf("DeleteLogDrain() error = %v", err)
	}

	_, err := st.GetLogDrain(ctx, drain.ID, drain.ProjectID)
	if !errors.Is(err, store.ErrLogDrainNotFound) {
		t.Fatalf("GetLogDrain() after delete error = %v, want ErrLogDrainNotFound", err)
	}

	err = st.DeleteLogDrain(ctx, drain.ID, drain.ProjectID)
	if !errors.Is(err, store.ErrLogDrainNotFound) {
		t.Fatalf("DeleteLogDrain() second call error = %v, want ErrLogDrainNotFound", err)
	}
}
