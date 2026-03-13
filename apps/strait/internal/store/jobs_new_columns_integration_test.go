//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"
)

func TestCreateJob_WithNewColumns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-job-new-cols")
	job.MaxConcurrencyPerKey = 5
	job.RateLimitKeys = []domain.RateLimitKey{{Name: "api", Max: 10, WindowSecs: 60}}
	job.DefaultRunMetadata = map[string]string{"env": "prod"}

	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}

	assertJobEqual(t, job, got)

	if got.MaxConcurrencyPerKey != 5 {
		t.Fatalf("MaxConcurrencyPerKey = %d, want 5", got.MaxConcurrencyPerKey)
	}
	if len(got.RateLimitKeys) != 1 {
		t.Fatalf("RateLimitKeys len = %d, want 1", len(got.RateLimitKeys))
	}
	if got.RateLimitKeys[0].Name != "api" {
		t.Fatalf("RateLimitKeys[0].Name = %q, want %q", got.RateLimitKeys[0].Name, "api")
	}
	if got.RateLimitKeys[0].Max != 10 {
		t.Fatalf("RateLimitKeys[0].Max = %d, want 10", got.RateLimitKeys[0].Max)
	}
	if got.RateLimitKeys[0].WindowSecs != 60 {
		t.Fatalf("RateLimitKeys[0].WindowSecs = %d, want 60", got.RateLimitKeys[0].WindowSecs)
	}
	if got.DefaultRunMetadata == nil {
		t.Fatal("DefaultRunMetadata is nil, want map with key \"env\"")
	}
	if got.DefaultRunMetadata["env"] != "prod" {
		t.Fatalf("DefaultRunMetadata[\"env\"] = %q, want %q", got.DefaultRunMetadata["env"], "prod")
	}
}

func TestUpdateJob_NewColumns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-update-new-cols")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	// Verify zero-valued defaults after initial create.
	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.MaxConcurrencyPerKey != 0 {
		t.Fatalf("initial MaxConcurrencyPerKey = %d, want 0", got.MaxConcurrencyPerKey)
	}
	if len(got.RateLimitKeys) != 0 {
		t.Fatalf("initial RateLimitKeys len = %d, want 0", len(got.RateLimitKeys))
	}
	if len(got.DefaultRunMetadata) != 0 {
		t.Fatalf("initial DefaultRunMetadata len = %d, want 0", len(got.DefaultRunMetadata))
	}

	// Set the new columns and update.
	job.MaxConcurrencyPerKey = 3
	job.RateLimitKeys = []domain.RateLimitKey{
		{Name: "user", Max: 100, WindowSecs: 3600},
		{Name: "ip", Max: 20, WindowSecs: 60},
	}
	job.DefaultRunMetadata = map[string]string{"region": "us-east-1", "tier": "premium"}

	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob() error = %v", err)
	}

	got, err = q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() after update error = %v", err)
	}

	if got.MaxConcurrencyPerKey != 3 {
		t.Fatalf("MaxConcurrencyPerKey = %d, want 3", got.MaxConcurrencyPerKey)
	}
	if len(got.RateLimitKeys) != 2 {
		t.Fatalf("RateLimitKeys len = %d, want 2", len(got.RateLimitKeys))
	}
	if got.RateLimitKeys[0].Name != "user" || got.RateLimitKeys[0].Max != 100 || got.RateLimitKeys[0].WindowSecs != 3600 {
		t.Fatalf("RateLimitKeys[0] = %+v, want {user 100 3600}", got.RateLimitKeys[0])
	}
	if got.RateLimitKeys[1].Name != "ip" || got.RateLimitKeys[1].Max != 20 || got.RateLimitKeys[1].WindowSecs != 60 {
		t.Fatalf("RateLimitKeys[1] = %+v, want {ip 20 60}", got.RateLimitKeys[1])
	}
	if len(got.DefaultRunMetadata) != 2 {
		t.Fatalf("DefaultRunMetadata len = %d, want 2", len(got.DefaultRunMetadata))
	}
	if got.DefaultRunMetadata["region"] != "us-east-1" {
		t.Fatalf("DefaultRunMetadata[\"region\"] = %q, want %q", got.DefaultRunMetadata["region"], "us-east-1")
	}
	if got.DefaultRunMetadata["tier"] != "premium" {
		t.Fatalf("DefaultRunMetadata[\"tier\"] = %q, want %q", got.DefaultRunMetadata["tier"], "premium")
	}
}

func TestUpdateJob_ClearNewColumns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-clear-new-cols")
	job.MaxConcurrencyPerKey = 10
	job.RateLimitKeys = []domain.RateLimitKey{{Name: "global", Max: 50, WindowSecs: 300}}
	job.DefaultRunMetadata = map[string]string{"source": "test"}

	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	// Clear all new columns.
	job.MaxConcurrencyPerKey = 0
	job.RateLimitKeys = nil
	job.DefaultRunMetadata = nil

	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob() error = %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() after clear error = %v", err)
	}

	if got.MaxConcurrencyPerKey != 0 {
		t.Fatalf("MaxConcurrencyPerKey = %d, want 0", got.MaxConcurrencyPerKey)
	}
	if len(got.RateLimitKeys) != 0 {
		t.Fatalf("RateLimitKeys len = %d, want 0 (got %v)", len(got.RateLimitKeys), got.RateLimitKeys)
	}
	if len(got.DefaultRunMetadata) != 0 {
		t.Fatalf("DefaultRunMetadata len = %d, want 0 (got %v)", len(got.DefaultRunMetadata), got.DefaultRunMetadata)
	}
}
