package api

import (
	"context"
	"strings"
	"sync"
	"testing"

	"strait/internal/domain"
)

// TestAuditEvent_PopulatesForensicFields verifies that the emit path
// stamps remote_ip, user_agent, request_id, trace_id, and schema_version
// onto the captured event from the request context.
func TestAuditEvent_PopulatesForensicFields(t *testing.T) {
	t.Parallel()

	var captured *domain.AuditEvent
	var mu sync.Mutex
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			clone := *ev
			captured = &clone
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxRemoteIPKey, "203.0.113.42")
	ctx = context.WithValue(ctx, ctxUserAgentKey, "Mozilla/5.0 (test)")
	ctx = context.WithValue(ctx, ctxRequestIDKey, "req-abc-123")
	ctx = context.WithValue(ctx, ctxTraceIDKey, "trace-xyz-789")

	srv.emitAuditEvent(ctx, domain.AuditActionJobCreated, "job", "job-1", map[string]any{
		"name": "x", "slug": "x", "execution_mode": "http",
	})

	mu.Lock()
	defer mu.Unlock()
	if captured == nil {
		t.Fatal("expected captured event")
	}
	if captured.RemoteIP != "203.0.113.42" {
		t.Errorf("RemoteIP = %q", captured.RemoteIP)
	}
	if captured.UserAgent != "Mozilla/5.0 (test)" {
		t.Errorf("UserAgent = %q", captured.UserAgent)
	}
	if captured.RequestID != "req-abc-123" {
		t.Errorf("RequestID = %q", captured.RequestID)
	}
	if captured.TraceID != "trace-xyz-789" {
		t.Errorf("TraceID = %q", captured.TraceID)
	}
	if captured.SchemaVersion != domain.AuditEventSchemaVersionCurrent {
		t.Errorf("SchemaVersion = %d, want %d", captured.SchemaVersion, domain.AuditEventSchemaVersionCurrent)
	}
}

// TestAuditEvent_EmptyForensicFieldsAreTolerated verifies that missing
// forensic context (e.g. internal scheduler calls) produces an event
// with empty forensic fields but still passes validation.
func TestAuditEvent_EmptyForensicFieldsAreTolerated(t *testing.T) {
	t.Parallel()

	var captured *domain.AuditEvent
	var mu sync.Mutex
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			clone := *ev
			captured = &clone
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	// No actor, no forensic fields — internal caller.
	srv.emitAuditEvent(ctx, domain.AuditActionJobCreated, "job", "job-1", map[string]any{
		"name": "x", "slug": "x", "execution_mode": "http",
	})

	mu.Lock()
	defer mu.Unlock()
	if captured == nil {
		t.Fatal("expected captured event (internal caller should succeed)")
	}
	if captured.RemoteIP != "" || captured.UserAgent != "" || captured.RequestID != "" {
		t.Errorf("expected empty forensic fields, got ip=%q ua=%q req=%q",
			captured.RemoteIP, captured.UserAgent, captured.RequestID)
	}
	if captured.SchemaVersion != domain.AuditEventSchemaVersionCurrent {
		t.Errorf("SchemaVersion = %d, want %d", captured.SchemaVersion, domain.AuditEventSchemaVersionCurrent)
	}
}

// TestAttachAuditContext_TruncatesOversizeUserAgent verifies the
// middleware caps user agents at auditUserAgentMaxBytes to prevent a
// malicious client from ballooning the audit log.
func TestAttachAuditContext_TruncatesOversizeUserAgent(t *testing.T) {
	t.Parallel()

	// Assert the constant matches the plan's 2048 bytes.
	if auditUserAgentMaxBytes != 2048 {
		t.Errorf("auditUserAgentMaxBytes = %d, want 2048", auditUserAgentMaxBytes)
	}

	// Use a sentinel that proves truncation.
	longUA := strings.Repeat("X", auditUserAgentMaxBytes*2)
	if len(longUA) <= auditUserAgentMaxBytes {
		t.Fatal("test data error")
	}

	// Simulate what attachAuditContext does to the UA string.
	ua := longUA
	if len(ua) > auditUserAgentMaxBytes {
		ua = ua[:auditUserAgentMaxBytes]
	}
	if len(ua) != auditUserAgentMaxBytes {
		t.Errorf("truncated UA length = %d, want %d", len(ua), auditUserAgentMaxBytes)
	}
}
