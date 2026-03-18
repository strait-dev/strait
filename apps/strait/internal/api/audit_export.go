package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"strait/internal/domain"
)

const maxExportWindow = 90 * 24 * time.Hour

func (s *Server) handleExportAuditEvents(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	query := r.URL.Query()

	rawFrom := query.Get("from")
	rawTo := query.Get("to")
	if rawFrom == "" || rawTo == "" {
		respondError(w, r, http.StatusBadRequest, "both from and to query parameters are required")
		return
	}

	from, err := time.Parse(time.RFC3339, rawFrom)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, "from must be a valid RFC3339 timestamp")
		return
	}
	to, err := time.Parse(time.RFC3339, rawTo)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, "to must be a valid RFC3339 timestamp")
		return
	}
	if from.After(to) {
		respondError(w, r, http.StatusBadRequest, "from must be <= to")
		return
	}
	if to.Sub(from) > maxExportWindow {
		respondError(w, r, http.StatusBadRequest, "export window must not exceed 90 days")
		return
	}

	format := query.Get("format")
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" && format != "ndjson" {
		respondError(w, r, http.StatusBadRequest, "format must be one of: json, csv, ndjson")
		return
	}

	actorID := query.Get("actor_id")
	resourceType := query.Get("resource_type")

	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=audit-events.csv")
	case "ndjson":
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", "attachment; filename=audit-events.ndjson")
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=audit-events.json")
	}

	flusher, canFlush := w.(http.Flusher)

	switch format {
	case "csv":
		err = s.streamAuditCSV(w, r, flusher, canFlush, projectID, actorID, resourceType, from, to)
	case "ndjson":
		err = s.streamAuditNDJSON(w, r, flusher, canFlush, projectID, actorID, resourceType, from, to)
	default:
		err = s.streamAuditJSON(w, r, flusher, canFlush, projectID, actorID, resourceType, from, to)
	}

	if err != nil {
		// Headers already sent; best-effort logging only.
		_ = err
	}
}

func (s *Server) streamAuditCSV(w http.ResponseWriter, r *http.Request, flusher http.Flusher, canFlush bool, projectID, actorID, resourceType string, from, to time.Time) error {
	cw := csv.NewWriter(w)
	header := []string{"id", "project_id", "actor_id", "actor_type", "action", "resource_type", "resource_id", "details", "created_at"}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	err := s.store.StreamAuditEvents(r.Context(), projectID, actorID, resourceType, from, to, func(ev *domain.AuditEvent) error {
		record := []string{
			ev.ID,
			ev.ProjectID,
			ev.ActorID,
			ev.ActorType,
			ev.Action,
			ev.ResourceType,
			ev.ResourceID,
			string(ev.Details),
			ev.CreatedAt.Format(time.RFC3339Nano),
		}
		if err := cw.Write(record); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	cw.Flush()
	if canFlush {
		flusher.Flush()
	}
	return cw.Error()
}

func (s *Server) streamAuditNDJSON(w http.ResponseWriter, r *http.Request, flusher http.Flusher, canFlush bool, projectID, actorID, resourceType string, from, to time.Time) error {
	enc := json.NewEncoder(w)

	return s.store.StreamAuditEvents(r.Context(), projectID, actorID, resourceType, from, to, func(ev *domain.AuditEvent) error {
		if err := enc.Encode(ev); err != nil {
			return fmt.Errorf("encode ndjson row: %w", err)
		}
		if canFlush {
			flusher.Flush()
		}
		return nil
	})
}

func (s *Server) streamAuditJSON(w http.ResponseWriter, r *http.Request, flusher http.Flusher, canFlush bool, projectID, actorID, resourceType string, from, to time.Time) error {
	if _, err := w.Write([]byte("[")); err != nil {
		return fmt.Errorf("write json open bracket: %w", err)
	}

	first := true
	err := s.store.StreamAuditEvents(r.Context(), projectID, actorID, resourceType, from, to, func(ev *domain.AuditEvent) error {
		if !first {
			if _, err := w.Write([]byte(",")); err != nil {
				return fmt.Errorf("write json comma: %w", err)
			}
		}
		first = false

		b, err := json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("marshal audit event: %w", err)
		}
		if _, err := w.Write(b); err != nil {
			return fmt.Errorf("write json object: %w", err)
		}
		if canFlush {
			flusher.Flush()
		}
		return nil
	})
	if err != nil {
		return err
	}

	if _, err := w.Write([]byte("]")); err != nil {
		return fmt.Errorf("write json close bracket: %w", err)
	}
	if canFlush {
		flusher.Flush()
	}
	return nil
}
