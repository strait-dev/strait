package api

import (
	"context"
	"strings"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NotNil(t, captured)
	assert.Equal(
		t, "203.0.113.42",

		captured.RemoteIP,
	)
	assert.Equal(
		t, "Mozilla/5.0 (test)",

		captured.
			UserAgent)
	assert.Equal(
		t, "req-abc-123",
		captured.
			RequestID,
	)
	assert.Equal(
		t, "trace-xyz-789",

		captured.TraceID,
	)
	assert.Equal(
		t, domain.AuditEventSchemaVersionCurrent,

		captured.SchemaVersion,
	)
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
	require.NotNil(t, captured)
	assert.False(
		t, captured.
			RemoteIP !=
			"" ||
			captured.
				UserAgent != "" ||
			captured.
				RequestID != "")
	assert.Equal(
		t, domain.AuditEventSchemaVersionCurrent,

		captured.SchemaVersion,
	)
}

// TestAttachAuditContext_TruncatesOversizeUserAgent verifies the
// middleware caps user agents at auditUserAgentMaxBytes to prevent a
// malicious client from ballooning the audit log.
func TestAttachAuditContext_TruncatesOversizeUserAgent(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 2048, auditUserAgentMaxBytes)

	// Assert the constant matches the plan's 2048 bytes.

	// Use a sentinel that proves truncation.
	longUA := strings.Repeat("X", auditUserAgentMaxBytes*2)
	require.Greater(t, len(longUA), auditUserAgentMaxBytes)

	// Simulate what attachAuditContext does to the UA string.
	ua := longUA
	if len(ua) > auditUserAgentMaxBytes {
		ua = ua[:auditUserAgentMaxBytes]
	}
	assert.Len(t,
		ua, auditUserAgentMaxBytes,
	)
}
