package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

const maxExportWindowDays = 90

type ExportJobsInput struct {
	Format string `query:"format"`
}
type ExportJobsOutput struct{ Body any }

func (s *Server) handleExportJobs(ctx context.Context, input *ExportJobsInput) (*ExportJobsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	format := input.Format
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "ndjson" {
		return nil, huma.Error400BadRequest("format must be one of: json, ndjson")
	}

	w := responseWriterFromContext(ctx)
	if w == nil {
		return nil, huma.Error500InternalServerError("internal error")
	}

	flusher, canFlush := w.(http.Flusher)

	// sanitizeJob strips secrets before export serialization.
	sanitizeJob := func(job *domain.Job) {
		job.WebhookSecret = ""
	}

	switch format {
	case "ndjson":
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", "attachment; filename=jobs.ndjson")
		enc := json.NewEncoder(w)
		if err := s.store.StreamJobs(ctx, projectID, func(job *domain.Job) error {
			sanitizeJob(job)
			if err := enc.Encode(job); err != nil {
				return err
			}
			if canFlush {
				flusher.Flush()
			}
			return nil
		}); err != nil {
			slog.Error("export stream interrupted", "type", "jobs", "project_id", projectID, "error", err)
		}
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=jobs.json")
		streamJSON(w, flusher, canFlush, func(write func(any) error) error {
			return s.store.StreamJobs(ctx, projectID, func(job *domain.Job) error {
				sanitizeJob(job)
				return write(job)
			})
		})
	}

	s.emitAuditEvent(ctx, domain.AuditActionJobsExported, "job", "", map[string]any{
		"format":     format,
		"project_id": projectID,
	})

	return nil, nil //nolint:nilnil // response has already been streamed to the Huma response writer
}

type ExportRunsInput struct {
	Format string `query:"format"`
	From   string `query:"from"`
	To     string `query:"to"`
}
type ExportRunsOutput struct{ Body any }

func (s *Server) handleExportRuns(ctx context.Context, input *ExportRunsInput) (*ExportRunsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	if input.From == "" || input.To == "" {
		return nil, huma.Error400BadRequest("both from and to query parameters are required")
	}

	from, err := time.Parse(time.RFC3339, input.From)
	if err != nil {
		return nil, huma.Error400BadRequest("from must be a valid RFC3339 timestamp")
	}
	to, err := time.Parse(time.RFC3339, input.To)
	if err != nil {
		return nil, huma.Error400BadRequest("to must be a valid RFC3339 timestamp")
	}
	if from.After(to) {
		return nil, huma.Error400BadRequest("from must be <= to")
	}
	if to.Sub(from) > time.Duration(maxExportWindowDays)*24*time.Hour {
		return nil, huma.Error400BadRequest(fmt.Sprintf("export window must not exceed %d days", maxExportWindowDays))
	}

	format := input.Format
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "ndjson" && format != "csv" {
		return nil, huma.Error400BadRequest("format must be one of: json, ndjson, csv")
	}

	w := responseWriterFromContext(ctx)
	if w == nil {
		return nil, huma.Error500InternalServerError("internal error")
	}

	flusher, canFlush := w.(http.Flusher)

	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=runs.csv")
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"id", "job_id", "status", "attempt", "triggered_by", "created_at", "started_at", "finished_at", "error"})
		if err := s.store.StreamRuns(ctx, projectID, from, to, func(run *domain.JobRun) error {
			startedAt := ""
			if run.StartedAt != nil {
				startedAt = run.StartedAt.Format(time.RFC3339Nano)
			}
			finishedAt := ""
			if run.FinishedAt != nil {
				finishedAt = run.FinishedAt.Format(time.RFC3339Nano)
			}
			return cw.Write([]string{
				run.ID, run.JobID, string(run.Status), strconv.Itoa(run.Attempt),
				run.TriggeredBy, run.CreatedAt.Format(time.RFC3339Nano),
				startedAt, finishedAt, run.Error,
			})
		}); err != nil {
			slog.Error("export stream interrupted", "type", "runs", "format", "csv", "project_id", projectID, "error", err)
		}
		cw.Flush()
		if canFlush {
			flusher.Flush()
		}
	case "ndjson":
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", "attachment; filename=runs.ndjson")
		enc := json.NewEncoder(w)
		if err := s.store.StreamRuns(ctx, projectID, from, to, func(run *domain.JobRun) error {
			if err := enc.Encode(run); err != nil {
				return err
			}
			if canFlush {
				flusher.Flush()
			}
			return nil
		}); err != nil {
			slog.Error("export stream interrupted", "type", "runs", "format", "ndjson", "project_id", projectID, "error", err)
		}
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=runs.json")
		streamJSON(w, flusher, canFlush, func(write func(any) error) error {
			return s.store.StreamRuns(ctx, projectID, from, to, func(run *domain.JobRun) error {
				return write(run)
			})
		})
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunsExported, "run", "", map[string]any{
		"format":     format,
		"from":       input.From,
		"to":         input.To,
		"project_id": projectID,
	})

	return nil, nil //nolint:nilnil // response has already been streamed to the Huma response writer
}

type ExportWorkflowsInput struct {
	Format string `query:"format"`
}
type ExportWorkflowsOutput struct{ Body any }

func (s *Server) handleExportWorkflows(ctx context.Context, input *ExportWorkflowsInput) (*ExportWorkflowsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	format := input.Format
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "ndjson" {
		return nil, huma.Error400BadRequest("format must be one of: json, ndjson")
	}

	w := responseWriterFromContext(ctx)
	if w == nil {
		return nil, huma.Error500InternalServerError("internal error")
	}

	flusher, canFlush := w.(http.Flusher)

	switch format {
	case "ndjson":
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", "attachment; filename=workflows.ndjson")
		enc := json.NewEncoder(w)
		if err := s.store.StreamWorkflows(ctx, projectID, func(wf *domain.Workflow) error {
			if err := enc.Encode(wf); err != nil {
				return err
			}
			if canFlush {
				flusher.Flush()
			}
			return nil
		}); err != nil {
			slog.Error("export stream interrupted", "type", "workflows", "project_id", projectID, "error", err)
		}
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=workflows.json")
		streamJSON(w, flusher, canFlush, func(write func(any) error) error {
			return s.store.StreamWorkflows(ctx, projectID, func(wf *domain.Workflow) error {
				return write(wf)
			})
		})
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowsExported, "workflow", "", map[string]any{
		"format":     format,
		"project_id": projectID,
	})

	return nil, nil //nolint:nilnil // response has already been streamed to the Huma response writer
}

// streamJSON writes a JSON array by streaming individual objects.
func streamJSON(w io.Writer, flusher http.Flusher, canFlush bool, iterate func(write func(any) error) error) {
	_, _ = w.Write([]byte("["))
	first := true
	_ = iterate(func(item any) error {
		if !first {
			_, _ = w.Write([]byte(","))
		}
		first = false
		b, err := json.Marshal(item)
		if err != nil {
			return err
		}
		_, _ = w.Write(b)
		if canFlush {
			flusher.Flush()
		}
		return nil
	})
	_, _ = w.Write([]byte("]"))
	if canFlush {
		flusher.Flush()
	}
}
