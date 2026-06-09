package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"
	"unicode"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/crypto/hkdf"
)

const maxExportWindow = 90 * 24 * time.Hour

// defaultMaxExportRows is the fallback row cap used when neither a
// per-project override (project_quotas.audit_export_row_cap) nor a
// server default (config.AuditExportRowCapDefault) is configured. If
// the stream hits the effective cap, the JSON/CSV/NDJSON writer emits
// a trailing {"_capped": true, "exported": N} marker and terminates
// cleanly. The caller can paginate with from/to.
//
// Tests that need a smaller cap should set config.AuditExportRowCapDefault
// on the server config rather than overriding this constant.
const defaultMaxExportRows int64 = 1_000_000

// resolveExportRowCap returns the row cap to apply to this export. The
// precedence is per-project override > configured default > package
// default. 0 at any level means "fall through to the next tier".
func (s *Server) resolveExportRowCap(ctx context.Context, projectID string) int64 {
	if rowCap, err := s.store.GetAuditExportRowCap(ctx, projectID); err == nil && rowCap > 0 {
		return rowCap
	}
	if s.config != nil && s.config.AuditExportRowCapDefault > 0 {
		return s.config.AuditExportRowCapDefault
	}
	return defaultMaxExportRows
}

// maxExportsPerProjectPerHour caps how many exports a single project
// can issue per hour. This is a DoS + data-leak rate limit — a
// compromised key with audit:export scope is bounded by this budget.
const maxExportsPerProjectPerHour = 10

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
	if environmentIDFromContext(ctx) != "" {
		return nil, huma.Error403Forbidden("audit export requires a project-wide key")
	}

	if err := s.checkFeatureAllowed(ctx, projectID, billing.FeatureAuditLogs, "Audit logs"); err != nil {
		return nil, err
	}

	// Rate limit: 10 exports per project per hour. Protects against a
	// compromised key with audit:export scope exfiltrating the log.
	// Uses AllowStrict (fail-closed): if Redis is down, deny the request
	// rather than silently pass it through. A downed rate-limit service
	// must not open the door for a compromised key to bulk-export the log.
	if s.rateLimiter == nil {
		slog.Error("audit export rate limiter is not configured; denying request", "project_id", projectID)
		return nil, huma.Error503ServiceUnavailable("rate limit service unavailable, please retry")
	}
	rlKey := fmt.Sprintf("audit_export:%s", projectID)
	result, rlErr := s.rateLimiter.AllowStrict(ctx, rlKey, maxExportsPerProjectPerHour, time.Hour)
	if rlErr != nil {
		slog.Error("audit export rate limit check failed, denying request", "project_id", projectID, "error", rlErr)
		return nil, huma.Error503ServiceUnavailable("rate limit service unavailable, please retry")
	}
	if !result.Allowed {
		return nil, huma.Error429TooManyRequests(
			fmt.Sprintf("audit export rate limit exceeded: max %d exports/hour/project", maxExportsPerProjectPerHour))
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

	format, err := normalizeExportFormat(input.Format, true, "format must be one of: json, csv, ndjson")
	if err != nil {
		return nil, err
	}

	w := responseWriterFromContext(ctx)
	r := requestFromContext(ctx)
	if w == nil || r == nil {
		return nil, huma.Error500InternalServerError("internal error")
	}

	actorID := input.ActorID
	resourceType := input.ResourceType
	auditDetails := map[string]any{
		"from":                 input.From,
		"to":                   input.To,
		"format":               format,
		"filter_actor":         input.ActorID,
		"filter_resource_type": input.ResourceType,
	}
	if err := s.createRequiredAuditEvent(ctx, domain.AuditActionAuditExported, "audit", projectID, auditDetails); err != nil {
		slog.Warn("failed to create required audit export event", "project_id", projectID, "error", err)
		if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
			s.metrics.AuditEventsDropped.Add(ctx, 1,
				metric.WithAttributes(attribute.String("reason", "required_write_failed")))
		}
		return nil, huma.Error500InternalServerError("failed to record audit export")
	}

	// Derive HMAC signing key if configured. InternalSecret (INTERNAL_SECRET)
	// is the correct source key: it is the platform auth key, which is the
	// appropriate material for signing audit exports. SecretEncryptionKey is
	// the envelope encryption key for secrets storage and must not be used
	// here — using the wrong key would tie export integrity to a key intended
	// for a different purpose.
	var mac hash.Hash
	signingEnabled := s.config.InternalSecret != ""
	if signingEnabled {
		signingKey, keyErr := deriveAuditSigningKey([]byte(s.config.InternalSecret))
		if keyErr != nil {
			return nil, huma.Error500InternalServerError("internal error")
		}
		mac = hmac.New(sha256.New, signingKey)
	}

	rowCap := s.resolveExportRowCap(ctx, projectID)

	if signingEnabled {
		// Buffer signed exports so the HMAC is delivered as a normal response
		// header. HTTP trailers — the only standards-compliant way to sign a
		// streamed body — are silently dropped by buffering reverse proxies
		// (nginx proxy_buffering on, ALB, CloudFront), which would strip the
		// integrity signature. This endpoint is admin-only and rate limited, and
		// exports are bounded by the row cap and 90-day window, so buffering is
		// acceptable; it also lets a mid-stream failure surface as a real 500
		// instead of a truncated, wrongly-signed body.
		var buf bytes.Buffer
		exported, capped, err := s.streamAuditByFormat(ctx, format, io.MultiWriter(&buf, mac), nil, false, projectID, actorID, resourceType, from, to, rowCap)
		if err != nil {
			slog.Warn("audit export generation error",
				"err", err, "project_id", projectID, "format", format,
				"rows_written", exported, "capped", capped)
			return nil, huma.Error500InternalServerError("failed to generate audit export")
		}
		sig := hex.EncodeToString(mac.Sum(nil))
		w.Header().Set("X-Audit-Signature", fmt.Sprintf("sha256=%s", sig))
		setExportFormatHeaders(w, format)
		if _, werr := w.Write(buf.Bytes()); werr != nil {
			slog.Warn("audit export write error", "err", werr, "project_id", projectID)
		}
		s.emitExportCappedIfNeeded(ctx, capped, projectID, exported, rowCap)
		return nil, nil
	}

	// Unsigned exports stream directly: there is no integrity header to deliver,
	// so incremental flushing and lower memory use win.
	setExportFormatHeaders(w, format)
	flusher, canFlush := w.(http.Flusher)
	exported, capped, err := s.streamAuditByFormat(ctx, format, w, flusher, canFlush, projectID, actorID, resourceType, from, to, rowCap)
	if err != nil {
		// Headers already sent; we cannot surface a new status code. Log with
		// enough context to correlate a truncated payload report with the
		// server-side failure.
		slog.Warn("audit export stream error after headers",
			"err", err, "project_id", projectID, "format", format,
			"rows_written", exported, "capped", capped)
	}
	s.emitExportCappedIfNeeded(ctx, capped, projectID, exported, rowCap)

	// Return nil to signal that the response was already written.
	return nil, nil
}

// emitExportCappedIfNeeded records the audit-export-capped event and metric when
// an export hit the row cap.
func (s *Server) emitExportCappedIfNeeded(ctx context.Context, capped bool, projectID string, exported int, rowCap int64) {
	if !capped {
		return
	}
	s.emitAuditEvent(context.WithoutCancel(ctx), domain.AuditActionAuditExportCapped, "audit", projectID, map[string]any{
		"exported": exported,
		"cap":      rowCap,
	})
	if s.metrics != nil && s.metrics.AuditEventsExportCapped != nil {
		s.metrics.AuditEventsExportCapped.Add(ctx, 1,
			metric.WithAttributes(attribute.String("project_id", projectID)))
	}
}

// setExportFormatHeaders writes Content-Type and Content-Disposition headers
// for the chosen export format.
func setExportFormatHeaders(w http.ResponseWriter, format string) {
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
}

// streamAuditByFormat dispatches to the correct per-format streaming writer.
func (s *Server) streamAuditByFormat(ctx context.Context, format string, out io.Writer, flusher http.Flusher, canFlush bool, projectID, actorID, resourceType string, from, to time.Time, rowCap int64) (int, bool, error) {
	switch format {
	case "csv":
		return s.streamAuditCSV(ctx, out, flusher, canFlush, projectID, actorID, resourceType, from, to, rowCap)
	case "ndjson":
		return s.streamAuditNDJSON(ctx, out, flusher, canFlush, projectID, actorID, resourceType, from, to, rowCap)
	default:
		return s.streamAuditJSON(ctx, out, flusher, canFlush, projectID, actorID, resourceType, from, to, rowCap)
	}
}

// errExportCapReached is a sentinel used internally to stop the store
// stream once maxExportRows have been written.
var errExportCapReached = fmt.Errorf("audit export row cap reached")

// exportFlushInterval controls how often streaming exports call
// http.Flusher.Flush(). Flushing on every row keeps the client's first-
// byte latency low but pays a syscall per row — at 1M rows that is
// 1M Write() + 1M Flush() round trips against the TCP stack with no
// batching benefit. The interval strikes a balance: the client sees
// the first batch immediately and then fresh data at most
// (interval * per-row CPU + DB time) apart, while the server amortizes
// socket writes.
const exportFlushInterval = 1000

func (s *Server) streamAuditCSV(ctx context.Context, w io.Writer, flusher http.Flusher, canFlush bool, projectID, actorID, resourceType string, from, to time.Time, rowCap int64) (int, bool, error) {
	cw := csv.NewWriter(w)
	header := []string{"id", "project_id", "actor_id", "actor_type", "action", "resource_type", "resource_id", "details", "created_at", "remote_ip", "user_agent", "request_id", "trace_id", "schema_version"}
	if err := cw.Write(header); err != nil {
		return 0, false, fmt.Errorf("write csv header: %w", err)
	}

	exported := 0
	capped := false
	err := s.store.StreamAuditEvents(ctx, projectID, actorID, resourceType, from, to, func(ev *domain.AuditEvent) error {
		if int64(exported) >= rowCap {
			capped = true
			return errExportCapReached
		}
		record := []string{
			sanitizeCSVCell(ev.ID),
			sanitizeCSVCell(ev.ProjectID),
			sanitizeCSVCell(ev.ActorID),
			sanitizeCSVCell(ev.ActorType),
			sanitizeCSVCell(ev.Action),
			sanitizeCSVCell(ev.ResourceType),
			sanitizeCSVCell(ev.ResourceID),
			sanitizeCSVCell(string(ev.Details)),
			ev.CreatedAt.Format(time.RFC3339Nano),
			sanitizeCSVCell(ev.RemoteIP),
			sanitizeCSVCell(ev.UserAgent),
			sanitizeCSVCell(ev.RequestID),
			sanitizeCSVCell(ev.TraceID),
			strconv.FormatUint(uint64(ev.SchemaVersion), 10),
		}
		if err := cw.Write(record); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
		exported++
		if canFlush && exported%exportFlushInterval == 0 {
			cw.Flush()
			if err := cw.Error(); err != nil {
				return fmt.Errorf("flush csv rows: %w", err)
			}
			flusher.Flush()
		}
		return nil
	})
	if err != nil && !errors.Is(err, errExportCapReached) {
		return exported, capped, err
	}
	if capped {
		// Append a CSV sentinel row noting the cap.
		_ = cw.Write([]string{"_capped", strconv.Itoa(exported), "", "", "", "", "", "", "", "", "", "", "", ""})
	}

	cw.Flush()
	if canFlush {
		flusher.Flush()
	}
	return exported, capped, cw.Error()
}

func (s *Server) streamAuditNDJSON(ctx context.Context, w io.Writer, flusher http.Flusher, canFlush bool, projectID, actorID, resourceType string, from, to time.Time, rowCap int64) (int, bool, error) {
	enc := json.NewEncoder(w)
	exported := 0
	capped := false

	err := s.store.StreamAuditEvents(ctx, projectID, actorID, resourceType, from, to, func(ev *domain.AuditEvent) error {
		if int64(exported) >= rowCap {
			capped = true
			return errExportCapReached
		}
		if err := enc.Encode(ev); err != nil {
			return fmt.Errorf("encode ndjson row: %w", err)
		}
		exported++
		if canFlush && exported%exportFlushInterval == 0 {
			flusher.Flush()
		}
		return nil
	})
	if err != nil && !errors.Is(err, errExportCapReached) {
		return exported, capped, err
	}
	if capped {
		_ = enc.Encode(map[string]any{"_capped": true, "exported": exported})
	}
	if canFlush {
		flusher.Flush()
	}
	return exported, capped, nil
}

func sanitizeCSVCell(value string) string {
	if value == "" {
		return value
	}
	// A literal control rune at index 0 (\t \r \n \x00) hides formula
	// triggers further down the cell from a human reviewer but still
	// lets spreadsheets parse the rest. Quote unconditionally; the
	// invisibles-skip pass below would otherwise drop these and miss the
	// danger when no =/+/-/@ follows.
	switch value[0] {
	case '\t', '\r', '\n', '\x00':
		return "'" + value
	}
	// Walk past leading invisible runes \u2014 format chars (Cf: BOM, ZWSP,
	// LRM, RLM, ZWJ, soft hyphen), combining marks, whitespace, and
	// other controls \u2014 and decide based on the first printable rune.
	for _, r := range value {
		if unicode.Is(unicode.Cf, r) || unicode.IsMark(r) ||
			unicode.IsSpace(r) || unicode.IsControl(r) {
			continue
		}
		switch r {
		case '=', '+', '-', '@':
			return "'" + value
		}
		return value
	}
	return value
}

func (s *Server) streamAuditJSON(ctx context.Context, w io.Writer, flusher http.Flusher, canFlush bool, projectID, actorID, resourceType string, from, to time.Time, rowCap int64) (int, bool, error) {
	if _, err := w.Write([]byte("[")); err != nil {
		return 0, false, fmt.Errorf("write json open bracket: %w", err)
	}

	first := true
	exported := 0
	capped := false
	err := s.store.StreamAuditEvents(ctx, projectID, actorID, resourceType, from, to, func(ev *domain.AuditEvent) error {
		if int64(exported) >= rowCap {
			capped = true
			return errExportCapReached
		}
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
		exported++
		if canFlush && exported%exportFlushInterval == 0 {
			flusher.Flush()
		}
		return nil
	})
	if err != nil && !errors.Is(err, errExportCapReached) {
		return exported, capped, err
	}
	if capped {
		// Emit a trailing object after a comma separator.
		if !first {
			_, _ = w.Write([]byte(","))
		}
		_, _ = w.Write(fmt.Appendf(nil, `{"_capped":true,"exported":%d}`, exported))
	}
	if _, err := w.Write([]byte("]")); err != nil {
		return exported, capped, fmt.Errorf("write json close bracket: %w", err)
	}
	if canFlush {
		flusher.Flush()
	}
	return exported, capped, nil
}

// deriveAuditSigningKey derives a 32-byte signing key from the master key
// using HKDF-SHA256 with the salt "audit-export-signing".
func deriveAuditSigningKey(masterKey []byte) ([]byte, error) {
	hkdfReader := hkdf.New(sha256.New, masterKey, []byte("audit-export-signing"), []byte("strait:v1:audit-export-hmac"))
	derived := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, derived); err != nil {
		return nil, fmt.Errorf("hkdf derive: %w", err)
	}
	return derived, nil
}
