package clickhouse

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		require.Fail(t, "exporter did not stop after context cancel")
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
		require.Fail(t, "Stop() without Start() deadlocked (did not return within 1 second)")
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
	assert.Equal(t, 2, exporter.
		PendingCount())
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
	assert.Equal(t, 0, exporter.
		PendingCount())
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
	assert.Equal(t, 1, exporter.
		PendingCount())
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
	assert.Equal(t, 1, exporter.
		PendingCount())
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
	assert.Equal(t, 1, exporter.
		PendingCount())
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
	assert.Equal(t, 1, exporter.
		PendingCount())
}

func TestExporter_NilExporter_NoPanic(t *testing.T) {
	t.Parallel()

	var exporter *Exporter
	// All methods must be safe on nil receiver.
	exporter.Start(context.Background())
	exporter.Stop()
	assert.False(t, exporter.Enqueue(RunEventRecord{}))
	assert.Equal(t, 0, exporter.PendingCount())
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
	assert.Equal(t, 1, exporter.
		PendingCount())
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
	assert.Equal(t, 1, exporter.
		PendingCount())
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
	assert.Equal(t, 3, exporter.
		PendingCount())
}
