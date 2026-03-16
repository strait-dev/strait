package authoring

import (
	"context"
	"errors"
	"testing"
)

type fakeJobClient struct {
	createFn      func(ctx context.Context, body map[string]any) (map[string]any, error)
	triggerFn     func(ctx context.Context, jobID string, body map[string]any) (map[string]any, error)
	bulkTriggerFn func(ctx context.Context, jobID string, body map[string]any) (map[string]any, error)
	getRunFn      func(ctx context.Context, runID string) (map[string]any, error)
}

func (f *fakeJobClient) CreateJob(ctx context.Context, body map[string]any) (map[string]any, error) {
	return f.createFn(ctx, body)
}
func (f *fakeJobClient) TriggerJob(ctx context.Context, jobID string, body map[string]any) (map[string]any, error) {
	return f.triggerFn(ctx, jobID, body)
}
func (f *fakeJobClient) BulkTriggerJob(ctx context.Context, jobID string, body map[string]any) (map[string]any, error) {
	return f.bulkTriggerFn(ctx, jobID, body)
}
func (f *fakeJobClient) GetRun(ctx context.Context, runID string) (map[string]any, error) {
	return f.getRunFn(ctx, runID)
}

type testPayload struct {
	SKU string `json:"sku"`
}

func TestDefineJob_Kind(t *testing.T) {
	job := DefineJob(JobOptions[testPayload]{
		Name:        "Test Job",
		Slug:        "test-job",
		EndpointURL: "https://worker.dev/jobs/test",
	})

	if job.Kind != "job" {
		t.Errorf("expected kind 'job', got %q", job.Kind)
	}
	if job.Slug != "test-job" {
		t.Errorf("expected slug 'test-job', got %q", job.Slug)
	}
}

func TestDefineJob_ToRegistrationBody(t *testing.T) {
	maxConc := 5
	maxAttempts := 3
	timeoutSecs := 300

	job := DefineJob(JobOptions[testPayload]{
		Name:           "Sync Inventory",
		Slug:           "sync-inventory",
		EndpointURL:    "https://worker.dev/jobs/sync",
		ProjectID:      "proj_1",
		Description:    "Syncs inventory",
		Cron:           "*/5 * * * *",
		Timezone:       "America/New_York",
		MaxConcurrency: &maxConc,
		MaxAttempts:    &maxAttempts,
		RetryStrategy:  "exponential",
		TimeoutSecs:    &timeoutSecs,
		Tags:           map[string]string{"team": "inventory"},
	})

	body, err := job.ToRegistrationBody("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if body["project_id"] != "proj_1" {
		t.Error("expected project_id")
	}
	if body["name"] != "Sync Inventory" {
		t.Error("expected name")
	}
	if body["slug"] != "sync-inventory" {
		t.Error("expected slug")
	}
	if body["endpoint_url"] != "https://worker.dev/jobs/sync" {
		t.Error("expected endpoint_url")
	}
	if body["cron"] != "*/5 * * * *" {
		t.Error("expected cron")
	}
	if body["max_concurrency"] != 5 {
		t.Error("expected max_concurrency")
	}
	if body["retry_strategy"] != "exponential" {
		t.Error("expected retry_strategy")
	}
}

func TestDefineJob_ToRegistrationBody_OverrideProjectID(t *testing.T) {
	job := DefineJob(JobOptions[testPayload]{
		Name:        "Test",
		Slug:        "test",
		EndpointURL: "https://worker.dev",
		ProjectID:   "proj_1",
	})

	body, err := job.ToRegistrationBody("proj_override")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["project_id"] != "proj_override" {
		t.Errorf("expected override project_id, got %v", body["project_id"])
	}
}

func TestDefineJob_ToRegistrationBody_MissingProjectID(t *testing.T) {
	job := DefineJob(JobOptions[testPayload]{
		Name:        "Test",
		Slug:        "test",
		EndpointURL: "https://worker.dev",
	})

	_, err := job.ToRegistrationBody("")
	if err == nil {
		t.Fatal("expected error for missing projectId")
	}
}

func TestDefineJob_Register(t *testing.T) {
	job := DefineJob(JobOptions[testPayload]{
		Name:        "Test",
		Slug:        "test",
		EndpointURL: "https://worker.dev",
		ProjectID:   "proj_1",
	})

	client := &fakeJobClient{
		createFn: func(_ context.Context, body map[string]any) (map[string]any, error) {
			if body["slug"] != "test" {
				t.Error("expected slug in body")
			}
			return map[string]any{"id": "job_123"}, nil
		},
	}

	result, err := job.Register(context.Background(), client, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "job_123" {
		t.Error("expected job_123")
	}
	if job.lastRegisteredJobID != "job_123" {
		t.Error("expected last registered ID to be set")
	}
}

func TestDefineJob_Trigger(t *testing.T) {
	job := DefineJob(JobOptions[testPayload]{
		Name:        "Test",
		Slug:        "test",
		EndpointURL: "https://worker.dev",
	})

	client := &fakeJobClient{
		triggerFn: func(_ context.Context, jobID string, body map[string]any) (map[string]any, error) {
			if jobID != "job_123" {
				t.Errorf("expected job_123, got %q", jobID)
			}
			payload, ok := body["payload"].(map[string]any)
			if !ok {
				t.Fatal("expected payload")
			}
			if payload["sku"] != "ABC-123" {
				t.Error("expected sku in payload")
			}
			return map[string]any{"id": "run_1", "status": "queued"}, nil
		},
	}

	result, err := job.Trigger(context.Background(), client, TriggerJobInput[testPayload]{
		JobID:   "job_123",
		Payload: testPayload{SKU: "ABC-123"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "run_1" {
		t.Error("expected run_1")
	}
}

func TestDefineJob_Trigger_UsesLastRegisteredID(t *testing.T) {
	job := DefineJob(JobOptions[testPayload]{
		Name:        "Test",
		Slug:        "test",
		EndpointURL: "https://worker.dev",
		ProjectID:   "proj_1",
	})

	client := &fakeJobClient{
		createFn: func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return map[string]any{"id": "job_auto"}, nil
		},
		triggerFn: func(_ context.Context, jobID string, _ map[string]any) (map[string]any, error) {
			if jobID != "job_auto" {
				t.Errorf("expected job_auto, got %q", jobID)
			}
			return map[string]any{"id": "run_1"}, nil
		},
	}

	_, _ = job.Register(context.Background(), client, "")

	result, err := job.Trigger(context.Background(), client, TriggerJobInput[testPayload]{
		Payload: testPayload{SKU: "test"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "run_1" {
		t.Error("expected run_1")
	}
}

func TestDefineJob_Trigger_NoJobID(t *testing.T) {
	job := DefineJob(JobOptions[testPayload]{
		Name:        "Test",
		Slug:        "test",
		EndpointURL: "https://worker.dev",
	})

	_, err := job.Trigger(context.Background(), &fakeJobClient{}, TriggerJobInput[testPayload]{
		Payload: testPayload{SKU: "test"},
	})

	if err == nil {
		t.Fatal("expected error for missing jobID")
	}
}

func TestDefineJob_Trigger_WithOptionalFields(t *testing.T) {
	job := DefineJob(JobOptions[testPayload]{
		Name:        "Test",
		Slug:        "test",
		EndpointURL: "https://worker.dev",
	})

	priority := 5
	dryRun := true

	client := &fakeJobClient{
		triggerFn: func(_ context.Context, _ string, body map[string]any) (map[string]any, error) {
			if body["idempotency_key"] != "idem_1" {
				t.Error("expected idempotency_key")
			}
			if body["priority"] != 5 {
				t.Error("expected priority")
			}
			if body["dry_run"] != true {
				t.Error("expected dry_run")
			}
			if body["scheduled_at"] != "2024-01-01T00:00:00Z" {
				t.Error("expected scheduled_at")
			}
			return map[string]any{"id": "run_1"}, nil
		},
	}

	_, err := job.Trigger(context.Background(), client, TriggerJobInput[testPayload]{
		JobID:          "job_1",
		Payload:        testPayload{SKU: "test"},
		IdempotencyKey: "idem_1",
		Priority:       &priority,
		DryRun:         &dryRun,
		ScheduledAt:    "2024-01-01T00:00:00Z",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefineJob_Register_Error(t *testing.T) {
	job := DefineJob(JobOptions[testPayload]{
		Name:        "Test",
		Slug:        "test",
		EndpointURL: "https://worker.dev",
		ProjectID:   "proj_1",
	})

	client := &fakeJobClient{
		createFn: func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return nil, errors.New("api error")
		},
	}

	_, err := job.Register(context.Background(), client, "")
	if err == nil || err.Error() != "api error" {
		t.Errorf("expected 'api error', got %v", err)
	}
}

func TestDefineJob_RunHandler(t *testing.T) {
	job := DefineJob(JobOptions[testPayload]{
		Name:        "Test",
		Slug:        "test",
		EndpointURL: "https://worker.dev",
		Run: func(p testPayload, ctx RunContext) (any, error) {
			return map[string]string{"processed": p.SKU}, nil
		},
	})

	if job.Run == nil {
		t.Fatal("expected Run handler")
	}

	result, err := job.Run(testPayload{SKU: "ABC"}, RunContext{RunID: "run_1", Attempt: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]string)
	if !ok || m["processed"] != "ABC" {
		t.Error("expected processed result")
	}
}
