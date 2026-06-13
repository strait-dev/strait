package api

import (
	"context"

	"strait/internal/domain"
)

// emitInternalSecretBypassAudit records that a project-scoped handler was
// entered via X-Internal-Secret without a project context. The audit row
// names the gate the caller skipped, the resource type and id touched, and
// (when available) the sender identity. This leaves a forensic trail when
// an internal secret is leaked: every entry into a handler that would
// normally require a project_id but was admitted via the internal-secret
// fallback produces an audit event a SOC reviewer can correlate.
//
// Callers should invoke this helper IMMEDIATELY after the
// `if projectID == "" && !isInternalCaller(ctx)` short-circuit guard has
// been crossed by an internal caller — i.e. when projectID is empty AND
// isInternalCaller(ctx) is true. The helper itself does not re-check that
// invariant; it records whatever the call site says happened.
//
// gate is a short stable identifier ("batch_enable_jobs",
// "send_event") naming the project-match check that was skipped.
// resourceType / resourceID identify the touched resource (use empty
// string when no specific resource id is available).
func (s *Server) emitInternalSecretBypassAudit(ctx context.Context, gate, handler, resourceType, resourceID string) {
	caller := bypassCallerLabel(ctx)
	// Use the async emitter: it runs on a background context with retries and a
	// dead-letter fallback. The previous synchronous emit forwarded the live
	// request context, so a client disconnect or transient DB error silently
	// dropped this security-relevant audit row — exactly the forensic trail this
	// helper exists to guarantee.
	s.emitAuditEventAsync(ctx, domain.AuditActionInternalSecretBypass, resourceType, resourceID, map[string]any{
		"gate":    gate,
		"caller":  caller,
		"handler": handler,
	})
}

func auditContextWithProject(ctx context.Context, projectID string) context.Context {
	if projectID == "" || projectIDFromContext(ctx) != "" {
		return ctx
	}
	return context.WithValue(ctx, ctxProjectIDKey, projectID)
}

func (s *Server) emitInternalSecretBypassAuditForProject(ctx context.Context, projectID, gate, handler, resourceType, resourceID string) {
	s.emitInternalSecretBypassAudit(auditContextWithProject(ctx, projectID), gate, handler, resourceType, resourceID)
}

// emitInternalSecretBypassAuditIfProjectless records successful authorization
// fallthroughs where the internal secret reached a project-owned resource
// without an API-key project context.
func (s *Server) emitInternalSecretBypassAuditIfProjectless(ctx context.Context, projectID, gate, handler, resourceType, resourceID string) {
	if projectIDFromContext(ctx) != "" || !isInternalCaller(ctx) {
		return
	}
	s.emitInternalSecretBypassAuditForProject(ctx, projectID, gate, handler, resourceType, resourceID)
}

// bypassCallerLabel returns the most specific identity available for an
// internal-secret bypass. By contract this is only called once the caller has
// crossed the internal-secret guard with an empty project context, so exactly
// two outcomes are reachable:
//  1. The authenticated actor id (user or api-key) when present.
//  2. "internal_secret" — only the X-Internal-Secret marker is set.
//
// The previously documented "api-key:<project_id>" and "unknown" branches were
// unreachable: every call site requires projectIDFromContext == "" and
// isInternalCaller == true, so a non-empty project id or a non-internal caller
// never reaches here.
func bypassCallerLabel(ctx context.Context) string {
	if id := actorFromContext(ctx); id != "" {
		return id
	}
	return "internal_secret"
}
