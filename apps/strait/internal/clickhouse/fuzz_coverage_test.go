//go:build !integration

package clickhouse

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// FuzzBuildConnURL exercises buildConnURL with arbitrary URL and database strings.
// The function must never panic regardless of input.
func FuzzBuildConnURL(f *testing.F) {
	// Seed corpus: valid and adversarial inputs.
	seeds := []struct {
		rawURL   string
		database string
	}{
		{"clickhouse://localhost:9000", "default"},
		{"clickhouse://localhost:9000?database=existing", "other"},
		{"", ""},
		{"", "mydb"},
		{"clickhouse://localhost:9000", ""},
		{"not-a-url", "db"},
		{"://", "db"},
		{"clickhouse://user:pass@host:9000/path?foo=bar", "analytics"},
		{"\x00\x01\x02", "db"},
		{"clickhouse://host:9000/" + string([]byte{0x7f}), "testdb"},
		{"http://[::1]:9000", "db"},
		{"clickhouse://host:9000?database=&extra=1", "db"},
		{"clickhouse://:@:0", ""},
		{string(make([]byte, 4096)), "db"},
	}

	for _, s := range seeds {
		f.Add(s.rawURL, s.database)
	}

	f.Fuzz(func(t *testing.T, rawURL, database string) {
		// buildConnURL must not panic on any input.
		_, _ = buildConnURL(rawURL, database)
	})
}

// FuzzExporterEnqueue exercises the Exporter.Enqueue path with arbitrary data
// embedded in various record types. The type switch in insertBatch must handle
// unknown types gracefully, and the backpressure path must not panic.
func FuzzExporterEnqueue(f *testing.F) {
	f.Add("event-1", "run-1", "proj-1", "job-1", "info", "msg", "{}")
	f.Add("", "", "", "", "", "", "")
	f.Add("\x00", "\xff", "a\nb", "tab\there", "<script>", "DROP TABLE", "{invalid json")
	f.Add(string(make([]byte, 8192)), "", "", "", "", "", "")

	f.Fuzz(func(t *testing.T, eventID, runID, projectID, jobID, level, message, metadata string) {
		logger := slog.Default()
		// Use a tiny batch size to exercise the backpressure / overflow path.
		exp := &Exporter{
			config:  ExporterConfig{BatchSize: 2, FlushInterval: time.Hour, Enabled: true},
			logger:  logger,
			pending: make([]any, 0, 2),
			stopCh:  make(chan struct{}),
			done:    make(chan struct{}),
		}

		// Enqueue a known record type.
		rec := RunEventRecord{
			EventID:   eventID,
			RunID:     runID,
			ProjectID: projectID,
			JobID:     jobID,
			EventType: "test",
			Level:     level,
			Message:   message,
			Metadata:  metadata,
			CreatedAt: time.Now(),
		}
		exp.Enqueue(rec)

		// Enqueue an unknown type (string) to exercise the default branch.
		exp.Enqueue(metadata)

		// Enqueue enough to trigger backpressure (batch size 2, overflow at 20).
		for range 25 {
			exp.Enqueue(rec)
		}

		// Verify PendingCount does not panic.
		_ = exp.PendingCount()
	})
}

// FuzzInsertBatchRecordTyping exercises the insertBatch type-switch with a mix
// of valid record types and arbitrary unknown types.
func FuzzInsertBatchRecordTyping(f *testing.F) {
	f.Add("run-id", "job-id", "proj-id", "completed", "standard", "small", 1, "api")
	f.Add("", "", "", "", "", "", 0, "")
	f.Add("\x00null\x00", "j\x00b", "p\nid", "status'quote", "mode\"dbl", "preset<html>", -1, "trigger\ttab")

	f.Fuzz(func(t *testing.T, runID, jobID, projectID, status, execMode, preset string, attempt int, triggeredBy string) {
		logger := slog.Default()
		// Exporter with nil client: insertBatch returns nil early, but
		// the type-switch grouping code still runs if we call it with a
		// non-nil client stub. We test the grouping path directly here.
		exp := &Exporter{
			config: ExporterConfig{BatchSize: 100, FlushInterval: time.Hour, Enabled: true},
			logger: logger,
		}

		now := time.Now()
		batch := []any{
			RunAnalyticsRecord{
				RunID:         runID,
				JobID:         jobID,
				ProjectID:     projectID,
				Status:        status,
				ExecutionMode: execMode,
				MachinePreset: preset,
				Attempt:       attempt,
				TriggeredBy:   triggeredBy,
				CreatedAt:     now,
			},
			ComputeUsageRecord{
				RunID:     runID,
				ProjectID: projectID,
				StartedAt: now,
			},
			JobMetadataRecord{
				JobID:     jobID,
				ProjectID: projectID,
				Slug:      status,
			},
			WebhookDeliveryEventRecord{
				DeliveryID: runID,
				ProjectID:  projectID,
				WebhookURL: triggeredBy,
				CreatedAt:  now,
			},
			WorkflowRunAnalyticsRecord{
				WorkflowRunID: runID,
				ProjectID:     projectID,
				CreatedAt:     now,
			},
			WorkflowStepAnalyticsRecord{
				StepRunID: runID,
				ProjectID: projectID,
				CreatedAt: now,
			},
			EventTriggerEventRecord{
				TriggerID: runID,
				ProjectID: projectID,
				CreatedAt: now,
			},
			// Unknown type to hit default case.
			struct{ X string }{X: status},
			42,
			nil,
		}

		// insertBatch with nil client returns nil immediately after grouping.
		_ = exp.insertBatch(context.Background(), batch)
	})
}

// FuzzIsShortPeriod exercises the isShortPeriod helper with arbitrary time
// values to ensure it never panics.
func FuzzIsShortPeriod(f *testing.F) {
	f.Add(int64(0), int64(0))
	f.Add(int64(0), int64(86400))
	f.Add(int64(-1000000), int64(1000000))
	f.Add(int64(1<<32), int64(1<<32+1))

	f.Fuzz(func(t *testing.T, fromUnix, toUnix int64) {
		from := time.Unix(fromUnix, 0)
		to := time.Unix(toUnix, 0)
		_ = isShortPeriod(from, to)
	})
}

// FuzzNewConfig exercises the New constructor with adversarial config values
// to verify it returns errors gracefully without panicking.
func FuzzNewConfig(f *testing.F) {
	f.Add("clickhouse://localhost:9000", "default", true, 10, 5)
	f.Add("", "", false, 0, 0)
	f.Add("not-a-url\x00with-null", "db\nname", true, -1, -1)
	f.Add("://broken", "", true, 1<<30, 1<<30)

	f.Fuzz(func(t *testing.T, url, database string, enabled bool, maxOpen, maxIdle int) {
		cfg := Config{
			URL:          url,
			Database:     database,
			Enabled:      enabled,
			MaxOpenConns: maxOpen,
			MaxIdleConns: maxIdle,
		}
		// New may return an error or nil client; it must never panic.
		c, err := New(cfg, nil)
		if err == nil && c != nil {
			_ = c.Close()
		}
	})
}
