package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"golang.org/x/crypto/hkdf"
)

const maxExportWindow = 90 * 24 * time.Hour

type ExportAuditEventsInput struct {
	From         string `query:"from"`
	To           string `query:"to"`
	Format       string `query:"format"`
	ActorID      string `query:"actor_id"`
	ResourceType string `query:"resource_type"`
}

// ExportAuditEventsOutput uses any Body because the handler streams raw CSV/NDJSON/JSON
// directly to the response writer. A nil return signals that the response was already written.
type ExportAuditEventsOutput struct {
	Body any
}

func (s *Server) handleExportAuditEvents(ctx context.Context, input *ExportAuditEventsInput) (*ExportAuditEventsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	if err := s.checkFeatureAllowed(ctx, projectID, billing.FeatureAuditLogs, "Audit logs"); err != nil {
		return nil, err
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
	if to.Sub(from) > maxExportWindow {
		return nil, huma.Error400BadRequest("export window must not exceed 90 days")
	}

	format := input.Format
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" && format != "ndjson" {
		return nil, huma.Error400BadRequest("format must be one of: json, csv, ndjson")
	}

	// Retrieve the raw response writer for streaming output.
	w := responseWriterFromContext(ctx)
	r := requestFromContext(ctx)
	if w == nil || r == nil {
		return nil, huma.Error500InternalServerError("internal error")
	}

	actorID := input.ActorID
	resourceType := input.ResourceType

	// Derive HMAC signing key if configured.
	var mac hash.Hash
	signingEnabled := s.config.SecretEncryptionKey != ""
	if signingEnabled {
		signingKey, keyErr := deriveAuditSigningKey([]byte(s.config.SecretEncryptionKey))
		if keyErr != nil {
			return nil, huma.Error500InternalServerError("internal error")
		}
		mac = hmac.New(sha256.New, signingKey)
	}

	if signingEnabled {
		w.Header().Set("Trailer", "X-Audit-Signature")
	}

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

	// Wrap writer to tee into HMAC hash.
	var out io.Writer = w
	if mac != nil {
		out = io.MultiWriter(w, mac)
	}

	flusher, canFlush := w.(http.Flusher)

	switch format {
	case "csv":
		err = s.streamAuditCSV(ctx, out, flusher, canFlush, projectID, actorID, resourceType, from, to)
	case "ndjson":
		err = s.streamAuditNDJSON(ctx, out, flusher, canFlush, projectID, actorID, resourceType, from, to)
	default:
		err = s.streamAuditJSON(ctx, out, flusher, canFlush, projectID, actorID, resourceType, from, to)
	}

	if err != nil {
		// Headers already sent; best-effort logging only.
		_ = err
	}

	// Set HMAC signature trailer after streaming.
	if mac != nil {
		sig := hex.EncodeToString(mac.Sum(nil))
		w.Header().Set("X-Audit-Signature", fmt.Sprintf("sha256=%s", sig))
	}

	s.emitAuditEvent(ctx, domain.AuditActionAuditExported, "audit", projectID, map[string]any{
		"from":          input.From,
		"to":            input.To,
		"format":        format,
		"filter_actor":  input.ActorID,
		"filter_resource_type": input.ResourceType,
	})

	// Return nil to signal that the response was already written.
	return nil, nil
}

func (s *Server) streamAuditCSV(ctx context.Context, w io.Writer, flusher http.Flusher, canFlush bool, projectID, actorID, resourceType string, from, to time.Time) error {
	cw := csv.NewWriter(w)
	header := []string{"id", "project_id", "actor_id", "actor_type", "action", "resource_type", "resource_id", "details", "created_at"}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	err := s.store.StreamAuditEvents(ctx, projectID, actorID, resourceType, from, to, func(ev *domain.AuditEvent) error {
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

func (s *Server) streamAuditNDJSON(ctx context.Context, w io.Writer, flusher http.Flusher, canFlush bool, projectID, actorID, resourceType string, from, to time.Time) error {
	enc := json.NewEncoder(w)

	return s.store.StreamAuditEvents(ctx, projectID, actorID, resourceType, from, to, func(ev *domain.AuditEvent) error {
		if err := enc.Encode(ev); err != nil {
			return fmt.Errorf("encode ndjson row: %w", err)
		}
		if canFlush {
			flusher.Flush()
		}
		return nil
	})
}

func (s *Server) streamAuditJSON(ctx context.Context, w io.Writer, flusher http.Flusher, canFlush bool, projectID, actorID, resourceType string, from, to time.Time) error {
	if _, err := w.Write([]byte("[")); err != nil {
		return fmt.Errorf("write json open bracket: %w", err)
	}

	first := true
	err := s.store.StreamAuditEvents(ctx, projectID, actorID, resourceType, from, to, func(ev *domain.AuditEvent) error {
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

// deriveAuditSigningKey derives a 32-byte signing key from the master key
// using HKDF-SHA256 with the salt "audit-export-signing".
func deriveAuditSigningKey(masterKey []byte) ([]byte, error) {
	hkdfReader := hkdf.New(sha256.New, masterKey, []byte("audit-export-signing"), nil)
	derived := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, derived); err != nil {
		return nil, fmt.Errorf("hkdf derive: %w", err)
	}
	return derived, nil
}
