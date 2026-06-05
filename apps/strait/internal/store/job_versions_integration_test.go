//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestCreateJobVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-create")

	v := &domain.JobVersion{
		ID:          newID(),
		JobID:       job.ID,
		Version:     1,
		VersionID:   "vid-1",
		Name:        job.Name,
		Slug:        job.Slug,
		Description: "first version",
		EndpointURL: "https://example.com/v1",
		MaxAttempts: 3,
		TimeoutSecs: 30,
	}
	if err := q.CreateJobVersion(ctx, v); err != nil {
		t.Fatalf("CreateJobVersion() error = %v", err)
	}
	if v.CreatedAt.IsZero() {
		t.Fatal("CreateJobVersion() did not set CreatedAt")
	}
}

func TestGetJobVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-get")

	v := &domain.JobVersion{
		ID:            newID(),
		JobID:         job.ID,
		Version:       1,
		Name:          job.Name,
		Slug:          job.Slug,
		EndpointURL:   "https://example.com/v1",
		MaxAttempts:   3,
		TimeoutSecs:   30,
		Tags:          map[string]string{"team": "core"},
		PayloadSchema: json.RawMessage(`{"type":"object"}`),
	}
	if err := q.CreateJobVersion(ctx, v); err != nil {
		t.Fatalf("CreateJobVersion() error = %v", err)
	}

	got, err := q.GetJobVersion(ctx, job.ID, 1)
	if err != nil {
		t.Fatalf("GetJobVersion() error = %v", err)
	}
	if got.ID != v.ID {
		t.Fatalf("GetJobVersion() id = %q, want %q", got.ID, v.ID)
	}
	if got.Tags["team"] != "core" {
		t.Fatalf("GetJobVersion() tags = %v, want team=core", got.Tags)
	}

	// Not found.
	_, err = q.GetJobVersion(ctx, job.ID, 99)
	if err == nil {
		t.Fatal("GetJobVersion(notfound) expected error, got nil")
	}
}

func TestListJobVersionsByJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-list")

	for i := 1; i <= 3; i++ {
		v := &domain.JobVersion{
			ID:          newID(),
			JobID:       job.ID,
			Version:     i,
			Name:        job.Name,
			Slug:        job.Slug,
			EndpointURL: "https://example.com/v",
			MaxAttempts: 3,
			TimeoutSecs: 30,
		}
		if err := q.CreateJobVersion(ctx, v); err != nil {
			t.Fatalf("CreateJobVersion(%d) error = %v", i, err)
		}
	}

	versions, err := q.ListJobVersionsByJob(ctx, job.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListJobVersionsByJob() error = %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("ListJobVersionsByJob() len = %d, want 3", len(versions))
	}
	// Should be ordered version DESC.
	if versions[0].Version != 3 || versions[2].Version != 1 {
		t.Fatalf("ListJobVersionsByJob() version order = [%d,%d,%d], want [3,2,1]",
			versions[0].Version, versions[1].Version, versions[2].Version)
	}

	// Empty.
	empty, err := q.ListJobVersionsByJob(ctx, newID(), 10, nil)
	if err != nil {
		t.Fatalf("ListJobVersionsByJob(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListJobVersionsByJob(empty) len = %d, want 0", len(empty))
	}
}

func TestGetJobVersionByVersionID_LookupByNanoid(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-by-vid")

	v := &domain.JobVersion{
		ID:          newID(),
		JobID:       job.ID,
		Version:     1,
		VersionID:   "nanoid-abc-123",
		Name:        job.Name,
		Slug:        job.Slug,
		EndpointURL: "https://example.com/v1",
		MaxAttempts: 3,
		TimeoutSecs: 30,
	}
	if err := q.CreateJobVersion(ctx, v); err != nil {
		t.Fatalf("CreateJobVersion() error = %v", err)
	}

	got, err := q.GetJobVersionByVersionID(ctx, "nanoid-abc-123")
	if err != nil {
		t.Fatalf("GetJobVersionByVersionID() error = %v", err)
	}
	if got.ID != v.ID {
		t.Fatalf("GetJobVersionByVersionID() id = %q, want %q", got.ID, v.ID)
	}

	// Not found.
	_, err = q.GetJobVersionByVersionID(ctx, "nonexistent")
	if !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("GetJobVersionByVersionID(notfound) error = %v, want ErrJobNotFound", err)
	}
}

func TestGetJobAtVersion_SnapshotAndFallback(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-at-version")

	v := &domain.JobVersion{
		ID:          newID(),
		JobID:       job.ID,
		Version:     1,
		Name:        "versioned-name",
		Slug:        job.Slug,
		EndpointURL: "https://example.com/v1",
		MaxAttempts: 3,
		TimeoutSecs: 30,
	}
	if err := q.CreateJobVersion(ctx, v); err != nil {
		t.Fatalf("CreateJobVersion() error = %v", err)
	}

	got, err := q.GetJobAtVersion(ctx, job.ID, 1)
	if err != nil {
		t.Fatalf("GetJobAtVersion() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetJobAtVersion() returned nil")
	}
	if got.Name != "versioned-name" {
		t.Fatalf("GetJobAtVersion() name = %q, want %q", got.Name, "versioned-name")
	}

	// Fallback for a version that does not exist should return the live job.
	fallback, err := q.GetJobAtVersion(ctx, job.ID, 999)
	if err != nil {
		t.Fatalf("GetJobAtVersion(fallback) error = %v", err)
	}
	if fallback.ID != job.ID {
		t.Fatalf("GetJobAtVersion(fallback) id = %q, want %q", fallback.ID, job.ID)
	}
}

func TestGetJobAtVersion_PreservesExecutionSnapshot(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-execution-snapshot")

	poisonPinned := 7
	job.Name = "pinned-execution-config"
	job.ExecutionMode = domain.ExecutionModeWorker
	job.Queue = "critical"
	job.PreferredRegions = []string{"iad1", "sfo1"}
	job.PoisonPillThreshold = &poisonPinned
	job.DebounceWindowSecs = 11
	job.BatchWindowSecs = 12
	job.BatchMaxSize = 13
	job.OnCompleteTriggerWorkflow = "workflow-pinned"
	job.OnCompleteTriggerJob = "job-pinned"
	job.OnCompletePayloadMapping = json.RawMessage(`{"complete":"pinned"}`)
	job.OnFailureTriggerJob = "job-failure-pinned"
	job.OnFailureTriggerWorkflow = "workflow-failure-pinned"
	job.OnFailurePayloadMapping = json.RawMessage(`{"failure":"pinned"}`)
	job.EndpointSigningSecret = "signing-secret-pinned"
	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob(pinned config) error = %v", err)
	}

	if err := q.PauseJob(ctx, job.ID, "versioned pause"); err != nil {
		t.Fatalf("PauseJob() error = %v", err)
	}
	job, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob(after pause) error = %v", err)
	}
	pinnedVersion := job.Version

	poisonLive := 3
	job.Name = "live-execution-config"
	job.ExecutionMode = domain.ExecutionModeHTTP
	job.Queue = "default"
	job.PreferredRegions = []string{"fra1"}
	job.PoisonPillThreshold = &poisonLive
	job.DebounceWindowSecs = 21
	job.BatchWindowSecs = 22
	job.BatchMaxSize = 23
	job.OnCompleteTriggerWorkflow = "workflow-live"
	job.OnCompleteTriggerJob = "job-live"
	job.OnCompletePayloadMapping = json.RawMessage(`{"complete":"live"}`)
	job.OnFailureTriggerJob = "job-failure-live"
	job.OnFailureTriggerWorkflow = "workflow-failure-live"
	job.OnFailurePayloadMapping = json.RawMessage(`{"failure":"live"}`)
	job.EndpointSigningSecret = "signing-secret-live"
	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob(live config) error = %v", err)
	}
	if err := q.ResumeJob(ctx, job.ID); err != nil {
		t.Fatalf("ResumeJob() error = %v", err)
	}

	got, err := q.GetJobAtVersion(ctx, job.ID, pinnedVersion)
	if err != nil {
		t.Fatalf("GetJobAtVersion(pinned) error = %v", err)
	}
	if got.Name != "pinned-execution-config" {
		t.Fatalf("Name = %q, want pinned-execution-config", got.Name)
	}
	if got.ExecutionMode != domain.ExecutionModeWorker {
		t.Fatalf("ExecutionMode = %q, want worker", got.ExecutionMode)
	}
	if got.Queue != "critical" {
		t.Fatalf("Queue = %q, want critical", got.Queue)
	}
	if !reflect.DeepEqual(got.PreferredRegions, []string{"iad1", "sfo1"}) {
		t.Fatalf("PreferredRegions = %#v, want [iad1 sfo1]", got.PreferredRegions)
	}
	if got.PoisonPillThreshold == nil || *got.PoisonPillThreshold != poisonPinned {
		t.Fatalf("PoisonPillThreshold = %v, want %d", got.PoisonPillThreshold, poisonPinned)
	}
	if got.DebounceWindowSecs != 11 || got.BatchWindowSecs != 12 || got.BatchMaxSize != 13 {
		t.Fatalf("batch/debounce = %d/%d/%d, want 11/12/13", got.DebounceWindowSecs, got.BatchWindowSecs, got.BatchMaxSize)
	}
	if got.OnCompleteTriggerWorkflow != "workflow-pinned" || got.OnCompleteTriggerJob != "job-pinned" {
		t.Fatalf("complete triggers = %q/%q, want pinned values", got.OnCompleteTriggerWorkflow, got.OnCompleteTriggerJob)
	}
	if !jsonEqual(got.OnCompletePayloadMapping, json.RawMessage(`{"complete":"pinned"}`)) {
		t.Fatalf("OnCompletePayloadMapping = %s, want pinned", string(got.OnCompletePayloadMapping))
	}
	if got.OnFailureTriggerWorkflow != "workflow-failure-pinned" || got.OnFailureTriggerJob != "job-failure-pinned" {
		t.Fatalf("failure triggers = %q/%q, want pinned values", got.OnFailureTriggerWorkflow, got.OnFailureTriggerJob)
	}
	if !jsonEqual(got.OnFailurePayloadMapping, json.RawMessage(`{"failure":"pinned"}`)) {
		t.Fatalf("OnFailurePayloadMapping = %s, want pinned", string(got.OnFailurePayloadMapping))
	}
	if !got.Paused || got.PauseReason != "versioned pause" || got.PausedAt == nil {
		t.Fatalf("pause snapshot = paused:%v reason:%q paused_at:%v, want pinned pause", got.Paused, got.PauseReason, got.PausedAt)
	}
	if got.EndpointSigningSecret != "signing-secret-pinned" {
		t.Fatalf("EndpointSigningSecret = %q, want pinned secret", got.EndpointSigningSecret)
	}
}

func TestUpdateJob_StaleVersionDoesNotCreateSnapshot(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-stale-snapshot")
	stale := *job

	job.Name = "winner"
	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob(winner) error = %v", err)
	}

	stale.Name = "stale"
	if err := q.UpdateJob(ctx, &stale); !errors.Is(err, store.ErrJobVersionConflict) {
		t.Fatalf("UpdateJob(stale) error = %v, want ErrJobVersionConflict", err)
	}

	var poisoned int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM job_versions WHERE job_id = $1 AND version = $2
	`, job.ID, job.Version).Scan(&poisoned); err != nil {
		t.Fatalf("count poisoned snapshots: %v", err)
	}
	if poisoned != 0 {
		t.Fatalf("stale update created %d snapshot(s) for live version %d, want 0", poisoned, job.Version)
	}

	current, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob(current) error = %v", err)
	}
	current.Name = "valid-after-stale"
	if err := q.UpdateJob(ctx, current); err != nil {
		t.Fatalf("UpdateJob(valid after stale) error = %v", err)
	}
	if current.Version != job.Version+1 {
		t.Fatalf("valid update version = %d, want %d", current.Version, job.Version+1)
	}
}

func TestUpdateJob_StaleVersionDoesNotBlockFutureSnapshot(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-stale-future")
	stale := *job

	job.Name = "winner-before-stale"
	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob(winner) error = %v", err)
	}
	winnerVersion := job.Version

	stale.Name = "stale-poison"
	if err := q.UpdateJob(ctx, &stale); !errors.Is(err, store.ErrJobVersionConflict) {
		t.Fatalf("UpdateJob(stale) error = %v, want ErrJobVersionConflict", err)
	}

	current, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob(current) error = %v", err)
	}
	current.Name = "valid-after-stale"
	if err := q.UpdateJob(ctx, current); err != nil {
		t.Fatalf("UpdateJob(valid after stale) error = %v", err)
	}

	versioned, err := q.GetJobAtVersion(ctx, job.ID, winnerVersion)
	if err != nil {
		t.Fatalf("GetJobAtVersion(winner version) error = %v", err)
	}
	if versioned.Name != "winner-before-stale" {
		t.Fatalf("versioned name = %q, want winner-before-stale", versioned.Name)
	}
	if current.Version != winnerVersion+1 {
		t.Fatalf("valid update version = %d, want %d", current.Version, winnerVersion+1)
	}
}
