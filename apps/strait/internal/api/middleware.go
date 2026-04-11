package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/ratelimit"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const ctxProjectIDKey contextKey = "project_id"
const ctxOrgIDKey contextKey = "org_id"
const ctxScopesKey contextKey = "scopes"
const ctxAPIKeyIDKey contextKey = "api_key_id"
const ctxActorIDKey contextKey = "actor_id"
const ctxActorTypeKey contextKey = "actor_type" // "user" or "api_key"
const ctxAuthKeyObjKey contextKey = "api_key_obj"

// Forensic metadata propagated through request context into audit events.
const ctxRemoteIPKey contextKey = "remote_ip"
const ctxUserAgentKey contextKey = "user_agent"
const ctxRequestIDKey contextKey = "request_id"
const ctxTraceIDKey contextKey = "trace_id"

// auditUserAgentMaxBytes caps the user agent string stored on each audit
// event. Real-world UAs are typically <200 chars; anything longer is
// almost certainly probing or pathological.
const auditUserAgentMaxBytes = 2048

// apiVersion is the current API version returned in response headers.
const apiVersion = "v1"

// requireJSONAccept returns 406 Not Acceptable if the client explicitly
// requests a content type the API cannot serve. Allows application/json,
// application/*, */*, and empty (default).
func requireJSONAccept(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")
		if accept != "" && accept != "*/*" {
			ok := false
			for part := range strings.SplitSeq(accept, ",") {
				mt := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
				if mt == "application/json" || mt == "application/*" || mt == "*/*" {
					ok = true
					break
				}
			}
			if !ok {
				respondError(w, r, http.StatusNotAcceptable, "this API only serves application/json")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requireJSONContentType returns 415 Unsupported Media Type if a mutation
// request (POST/PUT/PATCH) has a body but the Content-Type is not application/json.
func requireJSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			if r.ContentLength > 0 || r.Header.Get("Content-Type") != "" {
				ct := r.Header.Get("Content-Type")
				mt := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
				if mt != "application/json" {
					respondError(w, r, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// realIP extracts the client IP from the request, preferring the last entry
// in X-Forwarded-For (the one appended by the first trusted reverse proxy)
// over RemoteAddr. Returns only the IP, stripping port if present.
func realIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Use the rightmost (last) entry: this is the IP appended by the
		// first trusted proxy and cannot be spoofed by the client.
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[len(parts)-1])
		if ip != "" {
			return ip
		}
	}
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// sseTokenAuth extracts auth token from ?token= query param for SSE endpoints
// where browsers cannot set custom headers (EventSource API limitation).
// It first tries to parse the token as a short-lived SSE JWT (recommended).
// If that fails, it falls back to treating it as a raw API key (backward compatible).
func (s *Server) sseTokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Internal-Secret") != "" {
			next.ServeHTTP(w, r)
			return
		}
		token := r.URL.Query().Get("token")
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Try short-lived SSE JWT first (preferred path).
		if claims := s.parseSSEToken(token); claims != nil {
			ctx := context.WithValue(r.Context(), ctxProjectIDKey, claims.ProjectID)
			ctx = context.WithValue(ctx, ctxScopesKey, claims.Scopes)
			ctx = context.WithValue(ctx, ctxActorTypeKey, "sse_token")
			ctx = context.WithValue(ctx, ctxActorIDKey, "sse:"+claims.ProjectID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Fall back to raw API key in query param (backward compatible).
		r.Header.Set("Authorization", "Bearer "+token)
		next.ServeHTTP(w, r)
	})
}

func projectIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxProjectIDKey).(string); ok {
		return v
	}
	return ""
}

func remoteIPFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRemoteIPKey).(string); ok {
		return v
	}
	return ""
}

func userAgentFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxUserAgentKey).(string); ok {
		return v
	}
	return ""
}

func requestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRequestIDKey).(string); ok {
		return v
	}
	return ""
}

func traceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxTraceIDKey).(string); ok {
		return v
	}
	return ""
}

// attachAuditContext middleware stamps the request context with the
// forensic metadata (remote IP, user agent, request ID, trace ID) that
// audit events will later record. Installed early in the middleware
// chain so every downstream handler — even those that bypass auth —
// sees the same set of values.
func (s *Server) attachAuditContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = context.WithValue(ctx, ctxRemoteIPKey, realIP(r))
		ua := r.UserAgent()
		if len(ua) > auditUserAgentMaxBytes {
			ua = ua[:auditUserAgentMaxBytes]
		}
		ctx = context.WithValue(ctx, ctxUserAgentKey, ua)
		if reqID := chimw.GetReqID(ctx); reqID != "" {
			ctx = context.WithValue(ctx, ctxRequestIDKey, reqID)
		}
		// Trace ID: populated by OTel middleware if installed. We read
		// it via the span context to avoid a hard dependency on
		// otelchi's internal context keys.
		//
		// The span is empty when tracing is disabled — in that case
		// TraceID() returns an all-zero value and we leave the audit
		// field blank.
		if traceID := traceIDFromRequest(r); traceID != "" {
			ctx = context.WithValue(ctx, ctxTraceIDKey, traceID)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// traceIDFromRequest extracts the OTel trace ID from the request span,
// or the empty string if no span is active.
func traceIDFromRequest(r *http.Request) string {
	sc := oteltrace.SpanContextFromContext(r.Context())
	if !sc.IsValid() {
		return ""
	}
	tid := sc.TraceID()
	if !tid.IsValid() {
		return ""
	}
	return tid.String()
}

// errProjectMismatch is returned when a resource's project_id does not match the
// authenticated project. Handlers should map this to 404 (not 403) to avoid
// leaking the existence of cross-project resources.
var errProjectMismatch = errors.New("resource does not belong to the authenticated project")

// requireProjectMatch verifies that resourceProjectID matches the project in
// the request context. It returns errProjectMismatch when they differ. Internal
// callers that operate without a project context (scheduler, worker) should not
// use this helper.
func requireProjectMatch(ctx context.Context, resourceProjectID string) error {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil // internal caller without project context
	}
	if resourceProjectID != projectID {
		return errProjectMismatch
	}
	return nil
}

// requireRunAccess fetches the run by ID and enforces tenant isolation.
// Returns an appropriate huma error if the caller does not own the run.
// Internal callers (scheduler, worker) that operate without a project
// context skip the check.
func (s *Server) requireRunAccess(ctx context.Context, runID string) error {
	if projectIDFromContext(ctx) == "" {
		return nil // internal caller without project context
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return huma.Error404NotFound("run not found")
		}
		return huma.Error500InternalServerError("failed to get run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return huma.Error404NotFound("run not found")
	}
	return nil
}

func orgIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxOrgIDKey).(string); ok {
		return v
	}
	return ""
}

func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := realIP(r)

		// Check if this IP is locked out from too many failed attempts.
		if blocked, retryAfter := s.authLimiter.IsBlocked(r.Context(), clientIP); blocked {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			respondError(w, r, http.StatusTooManyRequests, ratelimit.BlockedError(retryAfter))
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer strait_") {
			s.authLimiter.RecordFailure(r.Context(), clientIP)
			respondError(w, r, http.StatusUnauthorized, "invalid or missing api key")
			return
		}

		rawKey := strings.TrimPrefix(authHeader, "Bearer ")
		keyHash := hashAPIKey(rawKey)

		apiKey, err := s.store.GetAPIKeyByHash(r.Context(), keyHash)
		if err != nil || apiKey == nil {
			s.authLimiter.RecordFailure(r.Context(), clientIP)
			respondError(w, r, http.StatusUnauthorized, "invalid api key")
			return
		}

		if apiKey.RevokedAt != nil {
			s.authLimiter.RecordFailure(r.Context(), clientIP)
			respondError(w, r, http.StatusUnauthorized, "api key has been revoked")
			return
		}

		now := time.Now()
		if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(now) {
			s.authLimiter.RecordFailure(r.Context(), clientIP)
			respondError(w, r, http.StatusUnauthorized, "api key has expired")
			return
		}
		if apiKey.GraceExpiresAt != nil && apiKey.GraceExpiresAt.Before(now) {
			s.authLimiter.RecordFailure(r.Context(), clientIP)
			respondError(w, r, http.StatusUnauthorized, "api key rotation grace period has ended")
			return
		}

		touchCtx := context.WithoutCancel(r.Context())
		s.bgPool.Submit(func() {
			ctx, cancel := context.WithTimeout(touchCtx, 2*time.Second)
			defer cancel()
			if err := s.store.TouchAPIKeyLastUsed(ctx, apiKey.ID); err != nil {
				slog.Error("failed to touch api key last used", "key_id", apiKey.ID, "error", err)
			}
		})

		ctx := context.WithValue(r.Context(), ctxProjectIDKey, apiKey.ProjectID)
		ctx = context.WithValue(ctx, ctxScopesKey, apiKey.Scopes)
		ctx = context.WithValue(ctx, ctxAPIKeyIDKey, apiKey.ID)
		ctx = context.WithValue(ctx, ctxAuthKeyObjKey, apiKey)

		// Org-scoped keys get org context set so cross-project queries work.
		if apiKey.OrgID != "" {
			ctx = context.WithValue(ctx, ctxOrgIDKey, apiKey.OrgID)
		}

		// Actor identity: API key requests are always attributed to the key itself.
		// User-level actor context is only set via internal secret auth (see below)
		// to prevent API key holders from impersonating users via X-Actor-Id headers.
		ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:"+apiKey.ID)
		ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) apiKeyOrSecretAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SSE token auth may have already authenticated via a short-lived JWT.
		if actorType, _ := r.Context().Value(ctxActorTypeKey).(string); actorType == "sse_token" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer strait_") {
			s.apiKeyAuth(next).ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(authHeader, "Bearer ") && s.oidcVerifier != nil && s.oidcVerifier.enabled {
			s.oidcAuth(next).ServeHTTP(w, r)
			return
		}

		s.internalSecretAuth(next).ServeHTTP(w, r)
	})
}

func (s *Server) oidcAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if token == "" {
			respondError(w, r, http.StatusUnauthorized, "missing bearer token")
			return
		}

		claims, err := s.oidcVerifier.verify(token)
		if err != nil {
			respondError(w, r, http.StatusUnauthorized, "invalid bearer token")
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, ctxActorIDKey, claims.Subject)
		ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
		// Use scopes from the JWT if present; otherwise fall back to empty
		// (non-nil) to signal the RBAC path in requirePermission. When scopes
		// are set from the token, they restrict what the user can do beyond
		// their database role — enforcing the principle of least privilege
		// from the OAuth consent screen.
		if tokenScopes := claims.Scopes(); tokenScopes != nil {
			ctx = context.WithValue(ctx, ctxScopesKey, tokenScopes)
		} else {
			ctx = context.WithValue(ctx, ctxScopesKey, []string{})
		}
		if projectID := strings.TrimSpace(r.Header.Get("X-Project-Id")); projectID != "" {
			if s.store == nil {
				respondError(w, r, http.StatusServiceUnavailable, "service unavailable")
				return
			}
			hasAccess, accessErr := s.store.UserHasProjectAccess(ctx, claims.Subject, projectID)
			if accessErr != nil {
				slog.Warn("failed to check project access", "user", claims.Subject, "project", projectID, "error", accessErr)
				respondError(w, r, http.StatusForbidden, "unable to verify project access")
				return
			}
			if !hasAccess {
				respondError(w, r, http.StatusForbidden, "no access to project")
				return
			}
			ctx = context.WithValue(ctx, ctxProjectIDKey, projectID)
		}

		if s.actorSyncer != nil {
			syncCtx := context.WithoutCancel(ctx)
			s.bgPool.Submit(func() {
				syncCtx2, cancel := context.WithTimeout(syncCtx, 2*time.Second)
				defer cancel()
				if err := s.actorSyncer.UpsertKnownActor(syncCtx2, claims.Subject, claims.Email, claims.Name); err != nil {
					slog.Warn("failed to sync actor from oidc", "actor_id", claims.Subject, "error", err)
				}
			})
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) internalSecretAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := realIP(r)

		// Check if this IP is locked out from too many failed attempts.
		if blocked, retryAfter := s.authLimiter.IsBlocked(r.Context(), clientIP); blocked {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			respondError(w, r, http.StatusTooManyRequests, ratelimit.BlockedError(retryAfter))
			return
		}

		secret := r.Header.Get("X-Internal-Secret")
		if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(s.config.InternalSecret)) != 1 {
			s.authLimiter.RecordFailure(r.Context(), clientIP)
			respondError(w, r, http.StatusUnauthorized, "invalid or missing internal secret")
			return
		}

		ctx := r.Context()

		// Optionally carry explicit project context for internal calls (e.g. RBAC management).
		// Check X-Project-Id header first, fall back to query param for backward compat.
		if projectID := strings.TrimSpace(r.Header.Get("X-Project-Id")); projectID != "" {
			ctx = context.WithValue(ctx, ctxProjectIDKey, projectID)
		} else if projectID := r.URL.Query().Get("project_id"); projectID != "" {
			ctx = context.WithValue(ctx, ctxProjectIDKey, projectID)
		}

		// Internal secret is trusted — extract actor identity from headers.
		// Only internal services (the app) can set X-Actor-Id to act on behalf of users.
		if actorID := r.Header.Get("X-Actor-Id"); actorID != "" {
			ctx = context.WithValue(ctx, ctxActorIDKey, actorID)
			ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

			if s.actorSyncer != nil {
				actorEmail := r.Header.Get("X-Actor-Email")
				actorName := r.Header.Get("X-Actor-Name")
				syncCtx := context.WithoutCancel(ctx)
				s.bgPool.Submit(func() {
					syncCtx2, cancel := context.WithTimeout(syncCtx, 2*time.Second)
					defer cancel()
					if err := s.actorSyncer.UpsertKnownActor(syncCtx2, actorID, actorEmail, actorName); err != nil {
						slog.Warn("failed to sync actor", "actor_id", actorID, "error", err)
					}
				})
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestLogger logs structured request/response information including
// method, path, query parameters, status, timing, and client metadata.
func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes", ww.BytesWritten(),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"request_id", chimw.GetReqID(r.Context()),
		}

		// Include sanitized query parameters (omit auth-related keys).
		if rawQuery := r.URL.RawQuery; rawQuery != "" {
			attrs = append(attrs, "query", sanitizeQuery(r.URL.Query()))
		}

		// Include Content-Length when present (useful for POST/PUT sizing).
		if r.ContentLength > 0 {
			attrs = append(attrs, "content_length", r.ContentLength)
		}

		// Log at appropriate level based on status code.
		status := ww.Status()
		switch {
		case status >= 500:
			slog.Error("request", attrs...)
		case status >= 400:
			slog.Warn("request", attrs...)
		default:
			slog.Info("request", attrs...)
		}
	})
}

func (s *Server) requestMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.metrics == nil {
			next.ServeHTTP(w, r)
			return
		}
		s.metrics.HTTPInflightRequests.Add(r.Context(), 1)
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		duration := time.Since(start).Seconds()
		s.metrics.HTTPRequestDuration.Record(context.Background(), duration,
			otelmetric.WithAttributes(attribute.Int("status", ww.Status())))
		s.metrics.HTTPInflightRequests.Add(context.Background(), -1)
	})
}

func apiKeyIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxAPIKeyIDKey).(string); ok {
		return v
	}
	return ""
}

func actorFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxActorIDKey).(string); ok {
		return v
	}
	return ""
}

func scopesFromContext(ctx context.Context) []string {
	if v, ok := ctx.Value(ctxScopesKey).([]string); ok {
		return v
	}
	return nil
}

// requirePermission returns a middleware that checks authorization based on
// the actor type. For API keys, it checks scopes. For users, it loads their
// role permissions from the database.
//
//nolint:gocognit
func (s *Server) requirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Check if request came through API key auth (scopes will be set).
			// Internal secret auth does not set scopes — those requests are
			// always allowed regardless of actor headers (actor is for audit only).
			scopes := scopesFromContext(ctx)
			if scopes == nil {
				// No scopes = internal secret auth — allow through.
				next.ServeHTTP(w, r)
				return
			}

			actorType, _ := ctx.Value(ctxActorTypeKey).(string)

			switch actorType {
			case "api_key", "sse_token":
				// API keys and SSE tokens use scopes directly.
				if domain.HasScope(scopes, permission) {
					next.ServeHTTP(w, r)
					return
				}
				respondError(w, r, http.StatusForbidden, "insufficient permissions: requires "+permission)
				return

			case "user":
				// Users always need a project context for permission checks.
				projectID := projectIDFromContext(ctx)
				actorID := actorFromContext(ctx)
				if projectID == "" || actorID == "" {
					respondError(w, r, http.StatusForbidden, "missing project or actor context")
					return
				}

				// OIDC tokens with explicit scopes: enforce the token's
				// granted scopes directly. This ensures the principle of
				// least privilege from the OAuth consent screen — the token
				// scopes restrict what the user can do, even if their
				// database role would allow more.
				if len(scopes) > 0 {
					if !domain.HasScope(scopes, permission) {
						respondError(w, r, http.StatusForbidden, "insufficient permissions: requires "+permission)
						return
					}
					// Token has the required scope — proceed.
					next.ServeHTTP(w, r)
					return
				}

				perms, cached := s.permCache.Get(projectID, actorID)
				if !cached {
					var err error
					perms, err = s.store.GetUserPermissions(ctx, projectID, actorID)
					if err != nil {
						respondError(w, r, http.StatusInternalServerError, "failed to load permissions")
						return
					}
					if perms != nil {
						s.permCache.Set(projectID, actorID, perms)
					}
				}

				if perms == nil {
					respondError(w, r, http.StatusForbidden, "no role assigned in this project")
					return
				}
				if domain.HasScope(perms, permission) {
					next.ServeHTTP(w, r)
					return
				}

				// Fallback: check resource-level policies.
				if resType, resID := resourceFromRequest(r); resType != "" && resID != "" {
					actions, rpErr := s.store.GetResourcePolicies(ctx, resType, resID, actorID)
					if rpErr == nil && domain.HasScope(actions, permission) {
						next.ServeHTTP(w, r)
						return
					}

					// Fallback: check tag-based policies for matching resources.
					if tags, ok := s.resourceTags(ctx, resType, resID); ok {
						tagActions, tpErr := s.store.GetTagPolicyActions(ctx, projectID, resType, actorID, tags)
						if tpErr == nil && domain.HasScope(tagActions, permission) {
							next.ServeHTTP(w, r)
							return
						}
					}
				}

				respondError(w, r, http.StatusForbidden, "insufficient permissions: requires "+permission)
				return

			default:
				respondError(w, r, http.StatusForbidden, "unknown actor type")
			}
		})
	}
}

// resourceFromRequest extracts the resource type and ID from the chi route context.
// Returns empty strings if the request doesn't target a specific resource.
func resourceFromRequest(r *http.Request) (string, string) {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return "", ""
	}

	params := rctx.URLParams
	for i, key := range params.Keys {
		switch key {
		case "jobID":
			return "job", params.Values[i]
		case "workflowID":
			return "workflow", params.Values[i]
		case "runID":
			return "run", params.Values[i]
		case "workflowRunID":
			return "workflow_run", params.Values[i]
		}
	}
	return "", ""
}

func (s *Server) resourceTags(ctx context.Context, resourceType, resourceID string) (map[string]string, bool) {
	switch resourceType {
	case "job":
		job, err := s.store.GetJob(ctx, resourceID)
		if err != nil || job == nil || len(job.Tags) == 0 {
			return nil, false
		}
		return job.Tags, true
	case "workflow":
		wf, err := s.store.GetWorkflow(ctx, resourceID)
		if err != nil || wf == nil || len(wf.Tags) == 0 {
			return nil, false
		}
		return wf.Tags, true
	default:
		return nil, false
	}
}

// projectContextMiddleware is retained for long-lived SSE routes that
// cannot safely hold a transaction open for the duration of the stream.
// For those routes the middleware is effectively a no-op for tenant
// isolation: the set_config call is transaction-local and is lost the
// moment its implicit transaction commits, so subsequent queries in the
// SSE handler run without a project context and fall back to
// application-level filtering. The main /v1 route group uses
// rlsTxMiddleware instead, which provides real RLS enforcement.
//
// TODO: tighten SSE isolation by making SSE handlers run their initial
// DB fetch inside store.WithTx + set_config and then release the tx
// before entering the long-running pub/sub loop.
func (s *Server) projectContextMiddleware(next http.Handler) http.Handler {
	setter, ok := s.store.(ProjectContextSetter)
	if !ok {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID := projectIDFromContext(r.Context())
		if projectID != "" {
			if err := setter.SetProjectContext(r.Context(), projectID); err != nil {
				slog.Warn("failed to set project context for RLS", "project_id", projectID, "error", err)
			}
			defer func() {
				if err := setter.ClearProjectContext(r.Context()); err != nil {
					slog.Warn("failed to clear project context for RLS", "error", err)
				}
			}()
		}
		next.ServeHTTP(w, r)
	})
}

// rlsTxMiddleware wraps each request in a per-request Postgres transaction
// and binds it to the request context via store.ContextWithTx. Every store
// method called during the request routes its queries through that tx
// (via the ctxAwareDBTX wrapper installed by store.NewWithContextRouting),
// which means the SELECT set_config('app.current_project_id', $1, true)
// call made at the start of the tx actually persists for every subsequent
// query in the request. Without this, RLS tenant isolation does not work
// under a connection pool because set_config's transaction-local setting
// is lost the moment an implicit transaction commits.
//
// The middleware fails closed: any error beginning the tx, setting the
// project context, or (after the handler runs) committing the tx results
// in a 500 response. A panic in the handler rolls the tx back and
// re-panics so the server's outer recovery middleware still handles it.
//
// Apply this middleware only to short-lived request handlers. Long-lived
// SSE or streaming routes continue to use projectContextMiddleware and
// rely on application-level filtering in the handler.
func (s *Server) rlsTxMiddleware(next http.Handler) http.Handler {
	if s.txPool == nil {
		// No tx pool configured (unit tests with a mock store). Fall
		// back to the legacy middleware so tests still exercise the
		// SetProjectContext code path.
		return s.projectContextMiddleware(next)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID := projectIDFromContext(r.Context())
		if projectID == "" {
			// Routes with no project context (public endpoints, health
			// checks) skip the tx wrap entirely.
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()
		tx, err := s.txPool.Begin(ctx)
		if err != nil {
			slog.Error("failed to begin RLS tx", "project_id", projectID, "error", err)
			respondError(w, r, http.StatusInternalServerError, "security context initialization failed")
			return
		}

		if _, err := tx.Exec(ctx, "SELECT set_config('app.current_project_id', $1, true)", projectID); err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, context.Canceled) {
				slog.Warn("failed to rollback RLS tx after set_config error", "error", rbErr)
			}
			slog.Error("failed to set RLS project context", "project_id", projectID, "error", err)
			respondError(w, r, http.StatusInternalServerError, "security context initialization failed")
			return
		}

		ctx = store.ContextWithTx(ctx, tx)

		// Track whether a panic occurred so we can rollback instead of
		// committing. Without this, a recovered panic could leave the
		// handler's partial writes committed.
		panicked := true
		defer func() {
			if panicked {
				if rbErr := tx.Rollback(context.Background()); rbErr != nil && !errors.Is(rbErr, context.Canceled) {
					slog.Warn("failed to rollback RLS tx after panic", "error", rbErr)
				}
			}
		}()

		next.ServeHTTP(w, r.WithContext(ctx))

		panicked = false
		if err := tx.Commit(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("failed to commit RLS tx", "project_id", projectID, "error", err)
		}
	})
}

// apiVersionHeader injects X-API-Version into every response.
func apiVersionHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-API-Version", apiVersion)
		next.ServeHTTP(w, r)
	})
}

// planUsageHeaders injects X-Strait-Plan and X-Strait-Usage-Limit into
// responses for authenticated API key requests. Uses cached plan limits
// from the billing enforcer, so no additional latency is added.
func (s *Server) planUsageHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Only for authenticated API key requests with a resolved project.
		scopes := scopesFromContext(ctx)
		projectID := projectIDFromContext(ctx)
		if len(scopes) == 0 || projectID == "" || s.billingEnforcer == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Skip for internal secret requests.
		if actorType, _ := ctx.Value(ctxActorTypeKey).(string); actorType == "internal" {
			next.ServeHTTP(w, r)
			return
		}

		limits := s.getOrgPlanLimits(ctx, projectID)
		if limits != nil {
			w.Header().Set("X-Strait-Plan", string(limits.PlanTier))
			s.setUsageHeaders(ctx, w, limits, projectID)
		}

		next.ServeHTTP(w, r)
	})
}

// setUsageHeaders writes X-Strait-Usage-Limit and X-Strait-Usage-Remaining.
func (s *Server) setUsageHeaders(ctx context.Context, w http.ResponseWriter, limits *billing.OrgPlanLimits, projectID string) {
	if limits.MaxRunsPerDay == -1 {
		w.Header().Set("X-Strait-Usage-Limit", "unlimited")
		w.Header().Set("X-Strait-Usage-Remaining", "unlimited")
		return
	}

	w.Header().Set("X-Strait-Usage-Limit", strconv.FormatInt(limits.MaxRunsPerDay, 10))

	orgID, _ := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if orgID == "" {
		return
	}

	used, err := s.billingEnforcer.GetDailyRunCount(ctx, orgID)
	if err != nil {
		return
	}

	remaining := max(limits.MaxRunsPerDay-used, 0)
	w.Header().Set("X-Strait-Usage-Remaining", strconv.FormatInt(remaining, 10))
}

type resolvedRateLimit struct {
	limit      int
	windowSecs int
	key        string
}

// resolveRateLimit determines the applicable rate limit by cascading through:
// API key override → project quota → plan-based → global defaults → per-IP.
func (s *Server) resolveRateLimit(ctx context.Context, r *http.Request, projectID, apiKeyID string) resolvedRateLimit {
	// 1. Try API key-level rate limit (from context, no DB hit).
	if apiKeyID != "" {
		if apiKey, ok := ctx.Value(ctxAuthKeyObjKey).(*domain.APIKey); ok && apiKey != nil && apiKey.RateLimitRequests > 0 && apiKey.RateLimitWindowSecs > 0 {
			return resolvedRateLimit{apiKey.RateLimitRequests, apiKey.RateLimitWindowSecs, "rl:apikey:" + apiKeyID}
		}
	}

	// 2. Fall back to project quota rate limit.
	if projectID != "" && s.store != nil {
		quota, err := s.store.GetProjectQuota(ctx, projectID)
		if err == nil && quota != nil && quota.RateLimitRequests > 0 && quota.RateLimitWindowSecs > 0 {
			return resolvedRateLimit{quota.RateLimitRequests, quota.RateLimitWindowSecs, "rl:project:" + projectID}
		}
	}

	// 3. Fall back to plan-based rate limit.
	if projectID != "" && s.billingEnforcer != nil {
		orgID, orgErr := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
		if orgErr == nil && orgID != "" {
			planLimits, limErr := s.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
			if limErr == nil && planLimits.APIRateLimit > 0 {
				return resolvedRateLimit{planLimits.APIRateLimit, 60, "rl:plan:" + orgID}
			}
		}
	}

	// 4. Fall back to global default rate limit per API key.
	if apiKeyID != "" && s.config.DefaultAPIKeyRateLimit > 0 {
		return resolvedRateLimit{s.config.DefaultAPIKeyRateLimit, s.config.DefaultAPIKeyRateWindowSecs, "rl:apikey:" + apiKeyID}
	}

	// 5. Fall back to global default rate limit per project.
	if projectID != "" && s.config.DefaultAPIKeyRateLimit > 0 {
		return resolvedRateLimit{s.config.DefaultAPIKeyRateLimit, s.config.DefaultAPIKeyRateWindowSecs, "rl:project:" + projectID}
	}

	// 6. Fall back to per-IP rate limit when no key/project limits matched.
	if s.config.RateLimitRequests > 0 {
		ip := realIP(r)
		return resolvedRateLimit{s.config.RateLimitRequests, int(time.Minute.Seconds()), "rl:ip:" + ip}
	}

	return resolvedRateLimit{}
}

// projectRateLimit enforces per-API-key and per-project rate limits using Redis.
func (s *Server) projectRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.rateLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()

		// Internal secret auth (no scopes set) bypasses rate limiting
		// so internal callers and stress tests are not throttled.
		if scopesFromContext(ctx) == nil && r.Header.Get("X-Internal-Secret") != "" {
			next.ServeHTTP(w, r)
			return
		}

		rl := s.resolveRateLimit(ctx, r, projectIDFromContext(ctx), apiKeyIDFromContext(ctx))
		if rl.limit == 0 {
			next.ServeHTTP(w, r)
			return
		}

		window := time.Duration(rl.windowSecs) * time.Second
		result, rlErr := s.rateLimiter.Allow(ctx, rl.key, rl.limit, window)
		if rlErr != nil {
			slog.Warn("rate limiter error, failing open", "key", rl.key, "error", rlErr)
		}
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
		if !result.Allowed {
			w.Header().Set("Retry-After", strconv.Itoa(rl.windowSecs))
			respondError(w, r, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// sanitizeQuery returns query parameter string with sensitive keys redacted.
func sanitizeQuery(params map[string][]string) string {
	sensitiveKeys := map[string]bool{
		"api_key": true,
		"token":   true,
		"secret":  true,
	}
	var sb strings.Builder
	first := true
	for k, vals := range params {
		for _, v := range vals {
			if !first {
				sb.WriteByte('&')
			}
			first = false
			sb.WriteString(k)
			sb.WriteByte('=')
			if sensitiveKeys[strings.ToLower(k)] {
				sb.WriteString("[REDACTED]")
			} else {
				sb.WriteString(v)
			}
		}
	}
	return sb.String()
}
