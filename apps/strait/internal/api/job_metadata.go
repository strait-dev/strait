package api

import (
	"strait/internal/clickhouse"
	"strait/internal/domain"
)

// enqueueJobMetadata sends a job metadata record to the ClickHouse exporter
// so the job_metadata table stays in sync with Postgres.
func (s *Server) enqueueJobMetadata(job *domain.Job) {
	if s.chExporter == nil || job == nil {
		return
	}
	s.chExporter.Enqueue(clickhouse.JobMetadataRecord{
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Slug:      job.Slug,
	})
}
