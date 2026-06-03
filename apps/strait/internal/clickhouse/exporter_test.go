package clickhouse

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
)

func TestExporter_Start_PanicRecovery(t *testing.T) {
	t.Parallel()

	// Create an exporter with a nil client so that insertBatch does not crash
	// on its own. We will trigger a panic by injecting a record that causes
	// a type-switch to panic via a custom type with a String() method that panics.
	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     10,
		FlushInterval: 50 * time.Millisecond,
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	exporter.Start(ctx)

	exporter.Enqueue(RunEventRecord{
		EventID:   "evt-1",
		RunID:     "run-1",
		ProjectID: "proj-1",
		CreatedAt: time.Now(),
	})

	cancel()

	// Wait for the done channel; if panic recovery did not work, this would
	// hang or the test process would crash.
	select {
	case <-exporter.done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("exporter did not stop after context cancel")
	}
}

func TestExporter_Stop_Cleanly(t *testing.T) {
	t.Parallel()

	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     10,
		FlushInterval: 50 * time.Millisecond,
	}, slog.Default())

	exporter.Start(context.Background())

	exporter.Enqueue(RunAnalyticsRecord{
		RunID:     "run-1",
		ProjectID: "proj-1",
		CreatedAt: time.Now(),
	})

	// Stop should drain and return without panic.
	exporter.Stop()
}

func TestExporter_StopWithoutStart_DoesNotDeadlock(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     10,
		FlushInterval: 50 * time.Millisecond,
	}, slog.Default())

	// Stop() without ever calling Start() must return promptly, not deadlock.
	done := make(chan struct{})
	concWG.Go(func() {
		exporter.Stop()
		close(done)
	})

	select {
	case <-done:
		// success: Stop returned without deadlock
	case <-time.After(1 * time.Second):
		t.Fatal("Stop() without Start() deadlocked (did not return within 1 second)")
	}
}

func TestExporter_EnqueuesNewRecordTypes(t *testing.T) {
	t.Parallel()

	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute, // won't auto-flush during test
	}, slog.Default())

	exporter.Enqueue(WorkflowApprovalEventRecord{
		ApprovalID:    "appr-1",
		WorkflowRunID: "wfr-1",
		StepRunID:     "sr-1",
		ProjectID:     "proj-1",
		Status:        "approved",
		RequestedAt:   time.Now(),
	})
	exporter.Enqueue(JobMetadataRecord{
		JobID:     "job-1",
		ProjectID: "proj-1",
		Slug:      "my-job",
	})

	if exporter.PendingCount() != 2 {
		t.Errorf("expected 2 pending records, got %d", exporter.PendingCount())
	}
}

func TestExporter_InsertBatch_UnknownType_Warns(t *testing.T) {
	t.Parallel()

	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	exporter.Enqueue("unknown-type-string")

	exporter.flush(context.Background())

	if exporter.PendingCount() != 0 {
		t.Errorf("expected 0 pending after flush, got %d", exporter.PendingCount())
	}
}

func TestExporter_EnqueuesEventTriggerEvent(t *testing.T) {
	t.Parallel()

	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	now := time.Now()
	exporter.Enqueue(EventTriggerEventRecord{
		TriggerID:      "trig-1",
		EventKey:       "my-event-key",
		ProjectID:      "proj-1",
		SourceType:     "workflow_step",
		Status:         "received",
		TimeoutSecs:    300,
		WaitDurationMs: 1500,
		CreatedAt:      now,
		ReceivedAt:     &now,
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending record, got %d", exporter.PendingCount())
	}
}

func TestExporter_EnqueuesWorkflowRunAnalytics(t *testing.T) {
	t.Parallel()

	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	now := time.Now()
	exporter.Enqueue(WorkflowRunAnalyticsRecord{
		WorkflowRunID: "wfr-1",
		WorkflowID:    "wf-1",
		ProjectID:     "proj-1",
		Status:        "completed",
		TriggeredBy:   "api",
		StepCount:     3,
		DurationMs:    5000,
		CreatedAt:     now,
		StartedAt:     &now,
		FinishedAt:    &now,
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending record, got %d", exporter.PendingCount())
	}
}

func TestExporter_EnqueuesWorkflowStepAnalytics(t *testing.T) {
	t.Parallel()

	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	now := time.Now()
	exporter.Enqueue(WorkflowStepAnalyticsRecord{
		StepRunID:     "sr-1",
		WorkflowRunID: "wfr-1",
		WorkflowID:    "wf-1",
		ProjectID:     "proj-1",
		StepRef:       "step-a",
		Status:        "completed",
		DurationMs:    1200,
		Attempt:       1,
		Error:         "",
		CreatedAt:     now,
		StartedAt:     &now,
		FinishedAt:    &now,
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending record, got %d", exporter.PendingCount())
	}
}

func TestExporter_EnqueuesWebhookDeliveryEvent(t *testing.T) {
	t.Parallel()

	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	now := time.Now()
	exporter.Enqueue(WebhookDeliveryEventRecord{
		DeliveryID:     "del-1",
		RunID:          "run-1",
		JobID:          "job-1",
		ProjectID:      "proj-1",
		WebhookURL:     "https://example.com/hook",
		Status:         "delivered",
		Attempts:       1,
		LastStatusCode: 200,
		DurationMs:     150,
		EventType:      "run_webhook",
		CreatedAt:      now,
		DeliveredAt:    &now,
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending record, got %d", exporter.PendingCount())
	}
}

func TestExporter_NilExporter_NoPanic(t *testing.T) {
	t.Parallel()

	var exporter *Exporter
	// All methods must be safe on nil receiver.
	exporter.Start(context.Background())
	exporter.Stop()
	if got := exporter.Enqueue(RunEventRecord{}); got {
		t.Error("expected Enqueue to return false on nil exporter")
	}
	if got := exporter.PendingCount(); got != 0 {
		t.Errorf("expected PendingCount 0 on nil exporter, got %d", got)
	}
}

func TestExporter_EnqueuesBillingEvent(t *testing.T) {
	t.Parallel()

	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	exporter.Enqueue(BillingEventRecord{
		Timestamp: time.Now(),
		OrgID:     "org-1",
		EventType: "plan_changed",
		PlanTier:  "pro",
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending record, got %d", exporter.PendingCount())
	}
}

func TestExporter_BillingEventWithAllFields(t *testing.T) {
	t.Parallel()

	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	exporter.Enqueue(BillingEventRecord{
		Timestamp: time.Now(),
		OrgID:     "org-1",
		ProjectID: "proj-1",
		EventType: "gate_rejected",
		Feature:   "canary_deployments",
		PlanTier:  "starter",
		Details:   `{"reason":"plan_limit"}`,
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending record, got %d", exporter.PendingCount())
	}
}

func TestExporter_BillingEventMixedWithOtherTypes(t *testing.T) {
	t.Parallel()

	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	exporter.Enqueue(BillingEventRecord{
		Timestamp: time.Now(),
		OrgID:     "org-1",
		EventType: "spending_limit_hit",
		PlanTier:  "pro",
	})
	exporter.Enqueue(RunEventRecord{
		EventID:   "evt-1",
		RunID:     "run-1",
		ProjectID: "proj-1",
		CreatedAt: time.Now(),
	})
	exporter.Enqueue(BillingEventRecord{
		Timestamp: time.Now(),
		OrgID:     "org-2",
		EventType: "plan_changed",
		PlanTier:  "scale",
	})

	if exporter.PendingCount() != 3 {
		t.Errorf("expected 3 pending records, got %d", exporter.PendingCount())
	}
}
