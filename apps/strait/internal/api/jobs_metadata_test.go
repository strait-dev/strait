package api

import (
	"log/slog"
	"testing"

	"strait/internal/clickhouse"
	"strait/internal/domain"
)

func TestEnqueueJobMetadata_WithExporter(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	srv := &Server{chExporter: exporter}

	job := &domain.Job{
		ID:        "job-1",
		ProjectID: "proj-1",
		Slug:      "my-job",
	}

	srv.enqueueJobMetadata(job)

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending job metadata record, got %d", exporter.PendingCount())
	}
}

func TestEnqueueJobMetadata_NilExporter(t *testing.T) {
	t.Parallel()
	srv := &Server{chExporter: nil}

	// Should not panic with nil exporter.
	srv.enqueueJobMetadata(&domain.Job{
		ID:        "job-1",
		ProjectID: "proj-1",
		Slug:      "my-job",
	})
}

func TestEnqueueJobMetadata_NilJob(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	srv := &Server{chExporter: exporter}

	// Should not panic with nil job.
	srv.enqueueJobMetadata(nil)

	if exporter.PendingCount() != 0 {
		t.Errorf("expected 0 pending for nil job, got %d", exporter.PendingCount())
	}
}

func TestEnqueueJobMetadata_MultipleJobs(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	srv := &Server{chExporter: exporter}

	for i, slug := range []string{"job-a", "job-b", "job-c"} {
		srv.enqueueJobMetadata(&domain.Job{
			ID:        "job-" + string(rune('1'+i)),
			ProjectID: "proj-1",
			Slug:      slug,
		})
	}

	if exporter.PendingCount() != 3 {
		t.Errorf("expected 3 pending job metadata records, got %d", exporter.PendingCount())
	}
}
