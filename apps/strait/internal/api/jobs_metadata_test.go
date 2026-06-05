package api

import (
	"log/slog"
	"testing"

	"strait/internal/clickhouse"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
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
	assert.EqualValues(t, 1, exporter.
		PendingCount())

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
	assert.EqualValues(t, 0, exporter.
		PendingCount())

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
	assert.EqualValues(t, 3, exporter.
		PendingCount())

}
