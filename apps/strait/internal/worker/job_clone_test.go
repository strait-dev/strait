package worker

import (
	"encoding/json"
	"testing"

	"strait/internal/domain"
)

func TestCloneJob_Nil(t *testing.T) {
	t.Parallel()

	if cloneJob(nil) != nil {
		t.Fatal("cloneJob(nil) returned non-nil")
	}
}

func TestCloneJob_CopiesScalars(t *testing.T) {
	t.Parallel()

	job := &domain.Job{
		ID:                    "job-1",
		ProjectID:             "project-1",
		Version:               7,
		VersionID:             "version-7",
		EndpointURL:           "https://example.com",
		ExecutionMode:         domain.ExecutionModeWorker,
		MaxAttempts:           5,
		TimeoutSecs:           300,
		CacheVersion:          11,
		EndpointSigningSecret: "encrypted",
	}

	clone := cloneJob(job)

	if clone == job {
		t.Fatal("cloneJob returned original pointer")
	}
	if clone.ID != job.ID ||
		clone.ProjectID != job.ProjectID ||
		clone.Version != job.Version ||
		clone.VersionID != job.VersionID ||
		clone.EndpointURL != job.EndpointURL ||
		clone.ExecutionMode != job.ExecutionMode ||
		clone.MaxAttempts != job.MaxAttempts ||
		clone.TimeoutSecs != job.TimeoutSecs ||
		clone.CacheVersion != job.CacheVersion ||
		clone.EndpointSigningSecret != job.EndpointSigningSecret {
		t.Fatalf("scalar fields not preserved: got %+v want %+v", clone, job)
	}
}

func TestCloneJob_IsolatesMutableFields(t *testing.T) {
	t.Parallel()

	job := &domain.Job{
		Tags:                     map[string]string{"team": "ops"},
		DefaultRunMetadata:       map[string]string{"trace": "on"},
		RetryDelaysSecs:          []int{1, 2, 3},
		RateLimitKeys:            []domain.RateLimitKey{{Name: "customer", Max: 10, WindowSecs: 60}},
		PreferredRegions:         []string{"iad", "fra"},
		ResultSchema:             json.RawMessage(`{"type":"object"}`),
		OnCompletePayloadMapping: json.RawMessage(`{"result":"$.output"}`),
		OnFailurePayloadMapping:  json.RawMessage(`{"error":"$.error"}`),
	}

	clone := cloneJob(job)

	clone.Tags["team"] = "platform"
	clone.DefaultRunMetadata["trace"] = "off"
	clone.RetryDelaysSecs[0] = 99
	clone.RateLimitKeys[0].Name = "tenant"
	clone.PreferredRegions[0] = "sfo"
	clone.ResultSchema[0] = '['
	clone.OnCompletePayloadMapping[0] = '['
	clone.OnFailurePayloadMapping[0] = '['

	if job.Tags["team"] != "ops" {
		t.Fatalf("original Tags mutated: %v", job.Tags)
	}
	if job.DefaultRunMetadata["trace"] != "on" {
		t.Fatalf("original DefaultRunMetadata mutated: %v", job.DefaultRunMetadata)
	}
	if job.RetryDelaysSecs[0] != 1 {
		t.Fatalf("original RetryDelaysSecs mutated: %v", job.RetryDelaysSecs)
	}
	if job.RateLimitKeys[0].Name != "customer" {
		t.Fatalf("original RateLimitKeys mutated: %v", job.RateLimitKeys)
	}
	if job.PreferredRegions[0] != "iad" {
		t.Fatalf("original PreferredRegions mutated: %v", job.PreferredRegions)
	}
	if string(job.ResultSchema) != `{"type":"object"}` {
		t.Fatalf("original ResultSchema mutated: %s", job.ResultSchema)
	}
	if string(job.OnCompletePayloadMapping) != `{"result":"$.output"}` {
		t.Fatalf("original OnCompletePayloadMapping mutated: %s", job.OnCompletePayloadMapping)
	}
	if string(job.OnFailurePayloadMapping) != `{"error":"$.error"}` {
		t.Fatalf("original OnFailurePayloadMapping mutated: %s", job.OnFailurePayloadMapping)
	}
}
