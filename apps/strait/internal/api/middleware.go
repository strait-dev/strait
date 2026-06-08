package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
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
const ctxEnvironmentIDKey contextKey = "environment_id"
const ctxScopesKey contextKey = "scopes"
const ctxAPIKeyIDKey contextKey = "api_key_id"
const ctxActorIDKey contextKey = "actor_id"
const ctxActorTypeKey contextKey = "actor_type" // "user" or "api_key"
const ctxAuthKeyObjKey contextKey = "api_key_obj"
const ctxOIDCScopeClaimPresentKey contextKey = "oidc_scope_claim_present"
const ctxTxCompletionHooksKey contextKey = "tx_completion_hooks"

// ctxInternalCallerKey is set to true by internalSecretAuth after the
// X-Internal-Secret header passes constant-time comparison. It is the
// authoritative signal that a request was authenticated via the internal
// secret — unlike nil scopes, which is also true for unauthenticated
// requests that never reached any auth middleware.
const ctxInternalCallerKey contextKey = "internal_caller"

// Forensic metadata propagated through request context into audit events.
const ctxRemoteIPKey contextKey = "remote_ip"
const ctxUserAgentKey contextKey = "user_agent"
const ctxRequestIDKey contextKey = "request_id"
const ctxTraceIDKey contextKey = "trace_id"

// attrRequestID is the OTel span attribute key for the per-request ID. We
// use a vendor-namespaced key rather than `http.request_id` to avoid
// colliding with the OTel HTTP semantic conventions
// (https://opentelemetry.io/docs/specs/semconv/http/) if and when an
// official `http.request.id` attribute graduates from incubating status.
const attrRequestID = "strait.request_id"

// auditUserAgentMaxBytes caps the user agent string stored on each audit
// event. Real-world UAs are typically <200 chars; anything longer is
// almost certainly probing or pathological.
const auditUserAgentMaxBytes = 2048
const successRequestLogSampleModulo = 100

// apiVersion is the current API version returned in response headers.
const apiVersion = "v1"

// requireJSONAccept returns 406 Not Acceptable if the client explicitly
// requests a content type the API cannot serve. Allows application/json,
// application/*, */*, and empty (default).
func requireJSONAccept(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")
		if !acceptsJSONResponse(accept) {
			respondError(w, r, http.StatusNotAcceptable, "this API only serves application/json")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func acceptsJSONResponse(accept string) bool {
	if accept == "" || accept == "*/*" {
		return true
	}
	if accept == "application/json" {
		return true
	}
	for len(accept) > 0 {
		part := accept
		if comma := strings.IndexByte(accept, ','); comma >= 0 {
			part = accept[:comma]
			accept = accept[comma+1:]
		} else {
			accept = ""
		}
		if semi := strings.IndexByte(part, ';'); semi >= 0 {
			part = part[:semi]
		}
		switch strings.TrimSpace(part) {
		case "application/json", "application/*", "application/x-ndjson", "text/csv", "*/*":
			return true
		}
	}
	return false
}

// requireJSONContentType returns 415 Unsupported Media Type if a mutation
// request (POST/PUT/PATCH) has a body but the Content-Type is not application/json.
func requireJSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			ct := r.Header.Get("Content-Type")
			if r.ContentLength > 0 || ct != "" {
				if ct == "" {
					respondError(w, r, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
					return
				}
				if ct != "application/json" {
					mt, _, _ := strings.Cut(ct, ";")
					mt = strings.TrimSpace(mt)
					if mt != "application/json" {
						respondError(w, r, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
						return
					}
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// realIP extracts the client IP for rate-limit / lockout accounting.
//
// When trustedProxies is non-empty, it walks X-Forwarded-For from right to
// left, skipping entries whose IP is in the trusted-proxy set, and returns
// the first untrusted IP it finds (the real client). When trustedProxies is
// empty, X-Forwarded-For is ignored entirely and the connection's RemoteAddr
// is returned, because any client can append arbitrary XFF entries and a
// rightmost-entry policy would let attackers spoof the IP recorded for
// lockout (rotating it to evade auth-failure throttling).
func realIP(r *http.Request, trustedProxies []net.IPNet) string {
	remote := remoteAddrIP(r)
	if len(trustedProxies) == 0 {
		return remote
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remote
	}
	if !ipInNets(remote, trustedProxies) {
		// The connecting peer is not a trusted proxy; ignore its XFF.
		return remote
	}
	for len(xff) > 0 {
		idx := strings.LastIndexByte(xff, ',')
		raw := xff
		if idx >= 0 {
			raw = xff[idx+1:]
			xff = xff[:idx]
		} else {
			xff = ""
		}
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}
		if ipInNets(candidate, trustedProxies) {
			continue
		}
		return candidate
	}
	return remote
}

// rateLimitKeyByIP is an httprate.KeyFunc that derives the rate-limit bucket
// from the trusted-proxy-aware client IP, instead of httprate's default which
// can be spoofed by clients in deployments where the server sees X-Forwarded-For.
func (s *Server) rateLimitKeyByIP(r *http.Request) (string, error) {
	return clientIPFromRequest(r, s.trustedProxies), nil
}

// remoteAddrIP returns the IP portion of r.RemoteAddr, stripping the port if
// present. It correctly handles IPv6 forms like "[::1]:1234".
func remoteAddrIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func clientIPFromRequest(r *http.Request, trustedProxies []net.IPNet) string {
	if ip := remoteIPFromContext(r.Context()); ip != "" {
		return ip
	}
	return realIP(r, trustedProxies)
}

// ipInNets reports whether the IP literal ip belongs to any of the given
// CIDR ranges. Invalid IPs return false.
func ipInNets(ip string, nets []net.IPNet) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for i := range nets {
		if nets[i].Contains(parsed) {
			return true
		}
	}
	return false
}

// parseTrustedProxies converts a list of CIDR strings or bare IPs from
// configuration into net.IPNet ranges. Invalid entries are dropped; a
// warning is the caller's responsibility.
func parseTrustedProxies(entries []string) []net.IPNet {
	out := make([]net.IPNet, 0, len(entries))
	for _, raw := range entries {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, cidr, err := net.ParseCIDR(raw); err == nil {
			out = append(out, *cidr)
			continue
		}
		if ip := net.ParseIP(raw); ip != nil {
			mask := net.CIDRMask(32, 32)
			if ip.To4() == nil {
				mask = net.CIDRMask(128, 128)
			}
			out = append(out, net.IPNet{IP: ip, Mask: mask})
		}
	}
	return out
}

// sseTokenAuth extracts auth token from ?token= query param for SSE endpoints
// where browsers cannot set custom headers (EventSource API limitation).
// Query tokens must be short-lived SSE JWTs. Long-lived API keys must be sent
// in the Authorization header so they do not leak through URLs or access logs.
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
			recordAuthDecision(r.Context(), "jwt", "success")
			if claims.IssuedAt != nil {
				recordAuthTokenAge(r.Context(), "jwt", claims.IssuedAt.Time)
			}
			ctx := context.WithValue(r.Context(), ctxProjectIDKey, claims.ProjectID)
			if claims.EnvironmentID != "" {
				ctx = context.WithValue(ctx, ctxEnvironmentIDKey, claims.EnvironmentID)
			}
			ctx = context.WithValue(ctx, ctxScopesKey, claims.Scopes)
			ctx = context.WithValue(ctx, ctxActorTypeKey, "sse_token")
			ctx = context.WithValue(ctx, ctxActorIDKey, "sse:"+claims.ProjectID)
			s.serveWithSentryScope(next, w, r.WithContext(ctx))
			return
		}

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
		ctx = context.WithValue(ctx, ctxRemoteIPKey, realIP(r, s.trustedProxies))
		ua := r.UserAgent()
		if len(ua) > auditUserAgentMaxBytes {
			ua = ua[:auditUserAgentMaxBytes]
		}
		ctx = context.WithValue(ctx, ctxUserAgentKey, ua)
		if reqID := chimw.GetReqID(ctx); reqID != "" {
			ctx = context.WithValue(ctx, ctxRequestIDKey, reqID)
			// Stamp the request ID onto the active OTel span so trace
			// explorers (Grafana, Tempo, Honeycomb) can pivot to the log
			// line that carries the same value. The span is a no-op when
			// tracing is disabled, so this is safe to call unconditionally.
			if span := oteltrace.SpanFromContext(ctx); span.SpanContext().IsValid() {
				span.SetAttributes(attribute.String(attrRequestID, reqID))
			}
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

// errEnvironmentMismatch is returned when an API key is bound to an
// environment that differs from the resource's environment. Handlers
// should map this to 404 (not 403) to avoid leaking the existence of
// resources in other environments.
var errEnvironmentMismatch = errors.New("resource does not belong to the authenticated environment")

// environmentIDFromContext returns the EnvironmentID stamped onto the
// request by apiKeyAuth. Empty string means the caller is not bound to a
// specific environment and may access resources in any environment of
// their project.
func environmentIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxEnvironmentIDKey).(string); ok {
		return v
	}
	return ""
}

// requireEnvironmentMatch enforces that an environment-scoped API key
// can only access resources in its own environment. The conservative
// rule: if the caller is bound to env X, every resource it touches must
// also be bound to env X. Resources without an EnvironmentID (legacy or
// unset) are also rejected for env-scoped keys to prevent silent
// privilege escalation when keys are progressively rolled out.
// Project-wide keys (empty ctx env) skip the check.
func requireEnvironmentMatch(ctx context.Context, resourceEnvironmentID string) error {
	callerEnv := environmentIDFromContext(ctx)
	if callerEnv == "" {
		return nil
	}
	if resourceEnvironmentID != callerEnv {
		return errEnvironmentMismatch
	}
	return nil
}

func requireProjectWideScope(ctx context.Context, resource string) error {
	if environmentIDFromContext(ctx) == "" {
		return nil
	}
	return huma.Error403Forbidden(resource + " requires a project-wide key")
}

// requireRunAccess fetches the run by ID and enforces tenant isolation.
// Returns an appropriate huma error if the caller does not own the run.
// Internal callers (scheduler, worker) that operate without a project
// context skip the check.
func (s *Server) getRunForAccess(ctx context.Context, runID string) (*domain.JobRun, error) {
	if projectIDFromContext(ctx) == "" {
		run, err := s.getRunWithStatusReadModel(ctx, runID)
		if err != nil {
			if errors.Is(err, store.ErrRunNotFound) {
				return nil, huma.Error404NotFound("run not found")
			}
			return nil, huma.Error500InternalServerError("failed to get run")
		}
		return run, nil
	}
	run, err := s.getRunWithStatusReadModel(ctx, runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("run not found")
	}
	if env := environmentIDFromContext(ctx); env != "" {
		// Environment scoping: an env-bound key must not reach a run
		// whose owning job lives in a different environment, even when
		// the project matches. The debug bundle in particular embeds
		// raw events/payloads/outputs, so this gate prevents staging
		// keys from pulling production telemetry.
		job, jobErr := s.store.GetJob(ctx, run.JobID)
		if jobErr != nil || job == nil {
			return nil, huma.Error404NotFound("run not found")
		}
		if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
			return nil, huma.Error404NotFound("run not found")
		}
	}
	return run, nil
}

func (s *Server) requireRunAccess(ctx context.Context, runID string) error {
	_, accessErr := s.getRunForAccess(ctx, runID)
	return accessErr
}

func orgIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxOrgIDKey).(string); ok {
		return v
	}
	return ""
}

func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := clientIPFromRequest(r, s.trustedProxies)

		// Check if this IP is locked out from too many failed attempts.
		if blocked, retryAfter := s.authLimiter.IsBlockedScoped(r.Context(), clientIP, ratelimit.AuthScopeAPIKey); blocked {
			recordAuthDecision(r.Context(), "api_key", "throttled")
			recordAuthRateLimitThrottled(r.Context(), "api_key")
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			respondError(w, r, http.StatusTooManyRequests, ratelimit.BlockedError(retryAfter))
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer strait_") {
			s.authLimiter.RecordFailureScoped(r.Context(), clientIP, ratelimit.AuthScopeAPIKey)
			recordAuthDecision(r.Context(), "api_key", "failure")
			respondError(w, r, http.StatusUnauthorized, "invalid or missing api key")
			return
		}

		rawKey := strings.TrimPrefix(authHeader, "Bearer ")
		keyHash := hashAPIKey(rawKey)

		apiKey, err := s.lookupAPIKeyForAuth(r.Context(), keyHash)
		if err != nil || apiKey == nil {
			s.authLimiter.RecordFailureScoped(r.Context(), clientIP, ratelimit.AuthScopeAPIKey)
			recordAuthDecision(r.Context(), "api_key", "failure")
			respondError(w, r, http.StatusUnauthorized, "invalid api key")
			return
		}

		if apiKey.RevokedAt != nil {
			s.authLimiter.RecordFailureScoped(r.Context(), clientIP, ratelimit.AuthScopeAPIKey)
			recordAuthDecision(r.Context(), "api_key", "failure")
			respondError(w, r, http.StatusUnauthorized, "api key has been revoked")
			return
		}

		now := time.Now()
		if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(now) {
			s.authLimiter.RecordFailureScoped(r.Context(), clientIP, ratelimit.AuthScopeAPIKey)
			recordAuthDecision(r.Context(), "api_key", "failure")
			respondError(w, r, http.StatusUnauthorized, "api key has expired")
			return
		}
		if apiKey.GraceExpiresAt != nil && apiKey.GraceExpiresAt.Before(now) {
			s.authLimiter.RecordFailureScoped(r.Context(), clientIP, ratelimit.AuthScopeAPIKey)
			recordAuthDecision(r.Context(), "api_key", "failure")
			respondError(w, r, http.StatusUnauthorized, "api key rotation grace period has ended")
			return
		}

		// Successful auth — clear the brute-force counter for this IP so
		// a legitimate user who fat-fingered their key a few times before
		// finding the right one isn't held up by the lockout window.
		s.authLimiter.ResetScoped(r.Context(), clientIP, ratelimit.AuthScopeAPIKey)
		recordAuthDecision(r.Context(), "api_key", "success")
		recordAuthTokenAge(r.Context(), "api_key", apiKey.CreatedAt)

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

		// Environment-scoped keys carry their bound environment so resource
		// handlers can reject requests targeting a different environment
		// (see requireEnvironmentMatch). A key with no EnvironmentID has
		// project-wide reach and is unaffected.
		if apiKey.EnvironmentID != "" {
			ctx = context.WithValue(ctx, ctxEnvironmentIDKey, apiKey.EnvironmentID)
		}

		// Actor identity: API key requests are always attributed to the key itself.
		// User-level actor context is only set via internal secret auth (see below)
		// to prevent API key holders from impersonating users via X-Actor-Id headers.
		ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:"+apiKey.ID)
		ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")

		s.serveWithSentryScope(next, w, r.WithContext(ctx))
	})
}

func (s *Server) lookupAPIKeyForAuth(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	loader := func(loadCtx context.Context, hash string) (*domain.APIKey, error) {
		return s.store.GetAPIKeyByHash(loadCtx, hash)
	}
	if s.apiKeyCache == nil {
		return loader(ctx, keyHash)
	}
	return s.apiKeyCache.Get(ctx, keyHash, loader)
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
		clientIP := clientIPFromRequest(r, s.trustedProxies)

		if blocked, retryAfter := s.authLimiter.IsBlockedScoped(r.Context(), clientIP, ratelimit.AuthScopeOIDC); blocked {
			recordAuthDecision(r.Context(), "oidc", "throttled")
			recordAuthRateLimitThrottled(r.Context(), "oidc")
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			respondError(w, r, http.StatusTooManyRequests, ratelimit.BlockedError(retryAfter))
			return
		}

		authHeader := r.Header.Get("Authorization")
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if token == "" {
			s.authLimiter.RecordFailureScoped(r.Context(), clientIP, ratelimit.AuthScopeOIDC)
			recordAuthDecision(r.Context(), "oidc", "failure")
			respondError(w, r, http.StatusUnauthorized, "missing bearer token")
			return
		}

		claims, err := s.oidcVerifier.verify(token)
		if err != nil {
			s.authLimiter.RecordFailureScoped(r.Context(), clientIP, ratelimit.AuthScopeOIDC)
			recordAuthDecision(r.Context(), "oidc", "failure")
			respondError(w, r, http.StatusUnauthorized, "invalid bearer token")
			return
		}

		s.authLimiter.ResetScoped(r.Context(), clientIP, ratelimit.AuthScopeOIDC)
		recordAuthDecision(r.Context(), "oidc", "success")
		if claims.IssuedAt != nil {
			recordAuthTokenAge(r.Context(), "oidc", claims.IssuedAt.Time)
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
			ctx = context.WithValue(ctx, ctxOIDCScopeClaimPresentKey, true)
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

		s.serveWithSentryScope(next, w, r.WithContext(ctx))
	})
}

func (s *Server) internalSecretAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := clientIPFromRequest(r, s.trustedProxies)

		// Check if this IP is locked out from too many failed attempts.
		if blocked, retryAfter := s.authLimiter.IsBlockedScoped(r.Context(), clientIP, ratelimit.AuthScopeInternalSecret); blocked {
			recordAuthDecision(r.Context(), "internal_secret", "throttled")
			recordAuthRateLimitThrottled(r.Context(), "internal_secret")
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			respondError(w, r, http.StatusTooManyRequests, ratelimit.BlockedError(retryAfter))
			return
		}

		secret := internalSecretFromRequest(r)
		if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(s.config.InternalSecret)) != 1 {
			s.authLimiter.RecordFailureScoped(r.Context(), clientIP, ratelimit.AuthScopeInternalSecret)
			recordAuthDecision(r.Context(), "internal_secret", "failure")
			respondError(w, r, http.StatusUnauthorized, "invalid or missing internal secret")
			return
		}

		s.authLimiter.ResetScoped(r.Context(), clientIP, ratelimit.AuthScopeInternalSecret)
		recordAuthDecision(r.Context(), "internal_secret", "success")

		ctx := r.Context()
		// Mark the request as authenticated via internal secret. This flag is
		// checked by isInternalCaller() and requireAdmin(), and is set here —
		// after the ConstantTimeCompare above passes — so unauthenticated
		// requests (no secret, wrong secret) never have it.
		ctx = context.WithValue(ctx, ctxInternalCallerKey, true)

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

		s.serveWithSentryScope(next, w, r.WithContext(ctx))
	})
}

func internalSecretFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if secret := r.Header.Get("X-Internal-Secret"); secret != "" {
		return secret
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	const bearerPrefix = "Bearer "
	if len(auth) > len(bearerPrefix) && strings.EqualFold(auth[:len(bearerPrefix)], bearerPrefix) {
		return strings.TrimSpace(auth[len(bearerPrefix):])
	}
	return ""
}

// requestLogger logs structured request/response information including
// method, path, query parameters, status, timing, and client metadata.
func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		status := ww.Status()
		requestID := chimw.GetReqID(r.Context())
		if !shouldLogRequest(status, requestID) {
			return
		}

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes", ww.BytesWritten(),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"request_id", requestID,
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

func shouldLogRequest(status int, requestID string) bool {
	if status >= 400 {
		return true
	}
	if successRequestLogSampleModulo <= 1 {
		return true
	}
	if requestID == "" {
		return false
	}
	return fnv32aString(requestID)%successRequestLogSampleModulo == 0
}

func fnv32aString(s string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for i := range len(s) {
		h ^= uint32(s[i])
		h *= prime32
	}
	return h
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

// isInternalCaller returns true only when the request was authenticated via
// the X-Internal-Secret header. Checking this flag is more reliable than
// checking scopesFromContext(ctx) == nil because nil scopes are also present
// on unauthenticated requests that bypassed auth middleware entirely.
func isInternalCaller(ctx context.Context) bool {
	v, _ := ctx.Value(ctxInternalCallerKey).(bool)
	return v
}

// requireInternalSecretMiddleware is a chi middleware that enforces
// internal-secret-only access at the router layer. It returns 403 for any
// request that was not positively authenticated via the X-Internal-Secret
// header (i.e. isInternalCaller returns false). Applying this at the route
// group level provides defense-in-depth: even if a handler's own requireAdmin
// call were skipped or removed, the middleware layer still gates access.
func (s *Server) requireInternalSecretMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isInternalCaller(r.Context()) {
			respondError(w, r, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requirePermission returns a middleware that checks authorization based on
// the actor type. For API keys, it checks scopes. For users, it loads their
// role permissions from the database.
func (s *Server) requirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			decision := s.authorizePermissionRequest(r, permission)
			if decision.allowed {
				next.ServeHTTP(w, r)
				return
			}
			respondError(w, r, decision.status, decision.message)
		})
	}
}

type permissionDecision struct {
	allowed bool
	status  int
	message string
}

func allowPermission() permissionDecision {
	return permissionDecision{allowed: true}
}

func denyPermission(status int, message string) permissionDecision {
	return permissionDecision{status: status, message: message}
}

func (s *Server) authorizePermissionRequest(r *http.Request, permission string) permissionDecision {
	ctx := r.Context()
	scopes := scopesFromContext(ctx)
	if scopes == nil && isInternalCaller(ctx) {
		return allowPermission()
	}

	actorType, _ := ctx.Value(ctxActorTypeKey).(string)
	switch actorType {
	case "api_key", "sse_token":
		return authorizeAPIKeyPermission(actorType, scopes, permission)
	case "user":
		return s.authorizeUserPermission(r, scopes, permission)
	default:
		return denyPermission(http.StatusForbidden, "unknown actor type")
	}
}

func authorizeAPIKeyPermission(actorType string, scopes []string, permission string) permissionDecision {
	hasScope := domain.HasScope(scopes, permission)
	if actorType == "sse_token" {
		hasScope = domain.HasScopeStrict(scopes, permission)
	}
	if hasScope {
		return allowPermission()
	}
	return denyPermission(http.StatusForbidden, "insufficient permissions: requires "+permission)
}

func (s *Server) authorizeUserPermission(r *http.Request, scopes []string, permission string) permissionDecision {
	ctx := r.Context()
	projectID := projectIDFromContext(ctx)
	actorID := actorFromContext(ctx)
	if projectID == "" || actorID == "" {
		return denyPermission(http.StatusForbidden, "missing project or actor context")
	}
	if decision := authorizeOIDCScopedPermission(ctx, scopes, permission); !decision.allowed && decision.message != "" {
		return decision
	}

	perms, decision := s.userProjectPermissions(ctx, projectID, actorID)
	if !decision.allowed {
		return decision
	}
	if domain.HasScopeStrict(perms, permission) || s.resourcePolicyAllows(r, projectID, actorID, permission) {
		return allowPermission()
	}
	return denyPermission(http.StatusForbidden, "insufficient permissions: requires "+permission)
}

func authorizeOIDCScopedPermission(ctx context.Context, scopes []string, permission string) permissionDecision {
	if ctx.Value(ctxOIDCScopeClaimPresentKey) == true && len(scopes) == 0 {
		return denyPermission(http.StatusForbidden, "insufficient permissions: requires "+permission)
	}
	if len(scopes) > 0 && !domain.HasScope(scopes, permission) {
		return denyPermission(http.StatusForbidden, "insufficient permissions: requires "+permission)
	}
	return allowPermission()
}

func (s *Server) userProjectPermissions(ctx context.Context, projectID, actorID string) ([]string, permissionDecision) {
	perms, cached := s.permCache.GetContext(ctx, projectID, actorID)
	if !cached {
		var version int64
		var err error
		perms, version, err = s.loadUserPermissionsForCache(ctx, projectID, actorID)
		if err != nil {
			return nil, denyPermission(http.StatusInternalServerError, "failed to load permissions")
		}
		if perms != nil {
			s.permCache.SetWithVersionContext(ctx, projectID, actorID, perms, version)
		}
	}
	if perms == nil {
		return nil, denyPermission(http.StatusForbidden, "no role assigned in this project")
	}
	return perms, allowPermission()
}

func (s *Server) resourcePolicyAllows(r *http.Request, projectID, actorID, permission string) bool {
	if !s.isRBACLevelAllowed(r.Context(), projectID, "advanced") {
		return false
	}

	resType, resID := resourceFromRequest(r)
	if resType == "" || resID == "" {
		return false
	}
	ctx := r.Context()
	actions, err := s.store.GetResourcePolicies(ctx, projectID, resType, resID, actorID)
	if err == nil && domain.HasScopeStrict(actions, permission) {
		return true
	}
	tags, ok := s.resourceTags(ctx, resType, resID)
	if !ok {
		return false
	}
	tagActions, err := s.store.GetTagPolicyActions(ctx, projectID, resType, actorID, tags)
	return err == nil && domain.HasScopeStrict(tagActions, permission)
}

func (s *Server) requireAnyPermission(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, permission := range permissions {
				if s.hasProjectPermission(r.Context(), permission) {
					next.ServeHTTP(w, r)
					return
				}
			}
			respondError(w, r, http.StatusForbidden, "insufficient permissions")
		})
	}
}

func (s *Server) hasProjectPermission(ctx context.Context, permission string) bool {
	if scopesFromContext(ctx) == nil && isInternalCaller(ctx) {
		return true
	}

	scopes := scopesFromContext(ctx)
	switch actorTypeFromContext(ctx) {
	case "api_key":
		return domain.HasScope(scopes, permission)
	case "sse_token":
		return domain.HasScopeStrict(scopes, permission)
	case "user":
		projectID := projectIDFromContext(ctx)
		actorID := actorFromContext(ctx)
		if projectID == "" || actorID == "" {
			return false
		}
		if ctx.Value(ctxOIDCScopeClaimPresentKey) == true && len(scopes) == 0 {
			return false
		}
		if len(scopes) > 0 && !domain.HasScope(scopes, permission) {
			return false
		}
		perms, cached := s.permCache.GetContext(ctx, projectID, actorID)
		if !cached {
			var version int64
			var err error
			perms, version, err = s.loadUserPermissionsForCache(ctx, projectID, actorID)
			if err != nil {
				return false
			}
			if perms != nil {
				s.permCache.SetWithVersionContext(ctx, projectID, actorID, perms, version)
			}
		}
		return domain.HasScopeStrict(perms, permission)
	default:
		return false
	}
}

type versionedUserPermissionStore interface {
	GetUserPermissionsWithVersion(ctx context.Context, projectID, userID string) ([]string, int64, error)
}

func (s *Server) loadUserPermissionsForCache(ctx context.Context, projectID, actorID string) ([]string, int64, error) {
	key := projectID + "\x00" + actorID
	if versioned, ok := s.store.(versionedUserPermissionStore); ok {
		perms, version, err := versioned.GetUserPermissionsWithVersion(ctx, projectID, actorID)
		if err != nil {
			return nil, 0, err
		}
		if getter, ok := s.store.(cacheNamespaceVersionGetter); ok {
			if aggregateVersion, getErr := getter.GetCacheNamespaceVersion(ctx, permissionCacheNamespace, key); getErr == nil && aggregateVersion > version {
				version = aggregateVersion
			}
		}
		return perms, version, nil
	}
	perms, err := s.store.GetUserPermissions(ctx, projectID, actorID)
	if err != nil {
		return nil, 0, err
	}
	return perms, time.Now().UnixNano(), nil
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
// SSE handlers that need database reads should perform their initial fetch
// inside store.WithTx + set_config and release the transaction before entering
// the long-running pub/sub loop.
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
		if bypassRLSTxBuffer(r) {
			s.projectContextMiddleware(next).ServeHTTP(w, r)
			return
		}

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

		hooks := &txCompletionHooks{}
		ctx = context.WithValue(store.ContextWithTx(ctx, tx), ctxTxCompletionHooksKey, hooks)

		// Track whether a panic occurred so we can rollback instead of
		// committing. Without this, a recovered panic could leave the
		// handler's partial writes committed.
		panicked := true
		defer func() {
			if panicked {
				if rbErr := tx.Rollback(context.Background()); rbErr != nil && !errors.Is(rbErr, context.Canceled) {
					slog.Warn("failed to rollback RLS tx after panic", "error", rbErr)
				}
				hooks.runRollback(context.Background())
			}
		}()

		bw := newBufferedResponseWriter(maxRLSBufferedResponseBytes)
		next.ServeHTTP(bw, r.WithContext(ctx))

		panicked = false
		if err := bw.Err(); err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, context.Canceled) {
				slog.Warn("failed to rollback RLS tx after oversized response", "error", rbErr)
			}
			hooks.runRollback(context.Background())
			respondError(w, r, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		if err := tx.Commit(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("failed to commit RLS tx", "project_id", projectID, "error", err)
			hooks.runRollback(context.Background())
			respondError(w, r, http.StatusInternalServerError, "security context commit failed")
			return
		}
		hooks.runCommit(context.Background())
		bw.FlushTo(w)
	})
}

type txCompletionHooks struct {
	mu            sync.Mutex
	afterCommit   []func(context.Context)
	afterRollback []func(context.Context)
}

func registerTxCompletionHooks(ctx context.Context, afterCommit, afterRollback func(context.Context)) bool {
	hooks, ok := ctx.Value(ctxTxCompletionHooksKey).(*txCompletionHooks)
	if !ok || hooks == nil {
		return false
	}
	hooks.mu.Lock()
	defer hooks.mu.Unlock()
	if afterCommit != nil {
		hooks.afterCommit = append(hooks.afterCommit, afterCommit)
	}
	if afterRollback != nil {
		hooks.afterRollback = append(hooks.afterRollback, afterRollback)
	}
	return true
}

func (h *txCompletionHooks) runCommit(ctx context.Context) {
	if h == nil {
		return
	}
	h.mu.Lock()
	hooks := append([]func(context.Context){}, h.afterCommit...)
	h.mu.Unlock()
	for _, hook := range hooks {
		hook(ctx)
	}
}

func (h *txCompletionHooks) runRollback(ctx context.Context) {
	if h == nil {
		return
	}
	h.mu.Lock()
	hooks := append([]func(context.Context){}, h.afterRollback...)
	h.mu.Unlock()
	for _, hook := range hooks {
		hook(ctx)
	}
}

const maxRLSBufferedResponseBytes = 16 << 20

var errRLSBufferedResponseTooLarge = errors.New("response too large")

func bypassRLSTxBuffer(r *http.Request) bool {
	if r.URL == nil {
		return false
	}
	if r.Method == http.MethodGet && r.URL.Path == "/v1/audit-events/export" {
		return true
	}
	return r.Method == http.MethodPost && r.URL.Path == "/v1/webhooks/test"
}

type bufferedResponseWriter struct {
	header      http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
	maxBytes    int
	err         error
}

func newBufferedResponseWriter(maxBytes int) *bufferedResponseWriter {
	return &bufferedResponseWriter{header: make(http.Header), status: http.StatusOK, maxBytes: maxBytes}
}

func (w *bufferedResponseWriter) Header() http.Header {
	return w.header
}

func (w *bufferedResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.status = statusCode
	w.wroteHeader = true
}

func (w *bufferedResponseWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.maxBytes > 0 && w.body.Len()+len(p) > w.maxBytes {
		w.err = fmt.Errorf("%w: buffered response exceeds %d bytes", errRLSBufferedResponseTooLarge, w.maxBytes)
		return 0, w.err
	}
	return w.body.Write(p)
}

func (w *bufferedResponseWriter) Err() error {
	return w.err
}

func (w *bufferedResponseWriter) FlushTo(dst http.ResponseWriter) {
	for key, values := range w.header {
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}
	dst.WriteHeader(w.status)
	_, _ = dst.Write(w.body.Bytes())
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
		// Internal-secret callers carry no scopes and are short-circuited here.
		// (A separate actorType=="internal" guard previously followed this, but no
		// middleware ever sets that actor type, so it was dead code and removed.)
		if len(scopes) == 0 || projectID == "" || s.billingEnforcer == nil {
			next.ServeHTTP(w, r)
			return
		}

		limits, err := s.getOrgPlanLimits(ctx, projectID)
		if err == nil && limits != nil {
			w.Header().Set("X-Strait-Plan", string(limits.PlanTier))
			s.setUsageHeaders(ctx, w, limits, projectID)
		}

		next.ServeHTTP(w, r)
	})
}

// setUsageHeaders writes X-Strait-Usage-Limit and X-Strait-Usage-Remaining
// for the monthly orchestration-run allowance.
func (s *Server) setUsageHeaders(ctx context.Context, w http.ResponseWriter, limits *billing.OrgPlanLimits, projectID string) {
	if limits.MaxRunsPerMonth == -1 {
		w.Header().Set("X-Strait-Usage-Limit", "unlimited")
		w.Header().Set("X-Strait-Usage-Remaining", "unlimited")
		return
	}

	w.Header().Set("X-Strait-Usage-Limit", strconv.Itoa(limits.MaxRunsPerMonth))

	orgID, _ := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if orgID == "" {
		return
	}

	used, err := s.billingEnforcer.GetMonthlyRunCount(ctx, orgID)
	if err != nil {
		return
	}

	remaining := max(int64(limits.MaxRunsPerMonth)-used, 0)
	w.Header().Set("X-Strait-Usage-Remaining", strconv.FormatInt(remaining, 10))
}

type resolvedRateLimit struct {
	limit      int
	windowSecs int
	key        string
	err        error
}

// resolveRateLimit determines the applicable rate limit by cascading through:
// API key override → project quota → plan-based → global defaults → per-IP.
// When a plan cap exists, API-key and project overrides can only lower it.
func (s *Server) resolveRateLimit(ctx context.Context, r *http.Request, projectID, apiKeyID string) resolvedRateLimit {
	planLimit := 0
	planResolved := false
	planKey := ""
	if projectID != "" && s.billingEnforcer != nil {
		orgID, orgErr := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
		if orgErr != nil {
			return resolvedRateLimit{err: fmt.Errorf("resolve project organization for rate limit: %w", orgErr)}
		}
		if orgID != "" {
			limits, limErr := s.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
			if limErr != nil {
				return resolvedRateLimit{err: fmt.Errorf("resolve plan limit for rate limit: %w", limErr)}
			}
			planLimit = limits.APIRateLimit
			planResolved = true
			planKey = "rl:plan:" + orgID
		}
	}

	// 1. Try API key-level rate limit (from context, no DB hit).
	if apiKeyID != "" {
		if apiKey, ok := ctx.Value(ctxAuthKeyObjKey).(*domain.APIKey); ok && apiKey != nil && apiKey.RateLimitRequests > 0 && apiKey.RateLimitWindowSecs > 0 {
			return capResolvedRateLimit(
				resolvedRateLimit{limit: apiKey.RateLimitRequests, windowSecs: apiKey.RateLimitWindowSecs, key: "rl:apikey:" + apiKeyID},
				planLimit,
			)
		}
	}

	// 2. Fall back to project quota rate limit.
	if projectID != "" && s.quotaCache != nil {
		quota, err := s.quotaCache.Get(ctx, projectID)
		if err == nil && quota != nil && quota.RateLimitRequests > 0 && quota.RateLimitWindowSecs > 0 {
			return capResolvedRateLimit(
				resolvedRateLimit{limit: quota.RateLimitRequests, windowSecs: quota.RateLimitWindowSecs, key: "rl:project:" + projectID},
				planLimit,
			)
		}
	}

	// 3. Fall back to plan-based rate limit.
	if projectID != "" && planLimit > 0 {
		return resolvedRateLimit{limit: planLimit, windowSecs: 60, key: planKey}
	}
	if projectID != "" && planResolved && planLimit < 0 {
		return resolvedRateLimit{}
	}

	// 4. Fall back to global default rate limit per API key.
	if apiKeyID != "" && s.config.DefaultAPIKeyRateLimit > 0 {
		return resolvedRateLimit{limit: s.config.DefaultAPIKeyRateLimit, windowSecs: s.config.DefaultAPIKeyRateWindowSecs, key: "rl:apikey:" + apiKeyID}
	}

	// 5. Fall back to global default rate limit per project.
	if projectID != "" && s.config.DefaultAPIKeyRateLimit > 0 {
		return resolvedRateLimit{limit: s.config.DefaultAPIKeyRateLimit, windowSecs: s.config.DefaultAPIKeyRateWindowSecs, key: "rl:project:" + projectID}
	}

	// 6. Fall back to per-IP rate limit when no key/project limits matched.
	if s.config.RateLimitRequests > 0 {
		ip := clientIPFromRequest(r, s.trustedProxies)
		return resolvedRateLimit{limit: s.config.RateLimitRequests, windowSecs: int(time.Minute.Seconds()), key: "rl:ip:" + ip}
	}

	return resolvedRateLimit{}
}

func capResolvedRateLimit(rl resolvedRateLimit, planLimitPerMinute int) resolvedRateLimit {
	if planLimitPerMinute <= 0 || rl.limit <= 0 || rl.windowSecs <= 0 {
		return rl
	}
	planLimitForWindow := planLimitPerMinute * rl.windowSecs / 60
	if planLimitForWindow < 1 {
		planLimitForWindow = 1
	}
	if rl.limit > planLimitForWindow {
		rl.limit = planLimitForWindow
	}
	return rl
}

// auditVerifyRateLimit enforces a per-project rate limit on the audit chain
// verify endpoint. The default of 1 req/project/60s is plenty for SOC 2
// evidence generation while still preventing a single tenant from
// dominating verifier compute (chain replay is O(events) per call). When
// no rate limiter is configured (test paths or RedisClient absent) the
// middleware is a no-op so unit tests continue to pass.
//
// Internal-secret callers bypass — operations / smoke tests need to
// verify on demand without bumping into the per-tenant quota.
const (
	auditVerifyRateLimitRequests   = 1
	auditVerifyRateLimitWindowSecs = 60
)

func (s *Server) auditVerifyRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.rateLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()
		if isInternalCaller(ctx) {
			next.ServeHTTP(w, r)
			return
		}
		projectID := projectIDFromContext(ctx)
		if projectID == "" {
			next.ServeHTTP(w, r)
			return
		}
		key := "rl:audit_verify:" + projectID
		window := time.Duration(auditVerifyRateLimitWindowSecs) * time.Second
		result, rlErr := s.rateLimiter.AllowStrict(ctx, key, auditVerifyRateLimitRequests, window)
		if rlErr != nil {
			slog.Error("audit verify rate limiter error, failing closed",
				"key", key, "error", rlErr)
			respondError(w, r, http.StatusServiceUnavailable, "rate limit service unavailable")
			return
		}
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(auditVerifyRateLimitRequests))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
		if !result.Allowed {
			w.Header().Set("Retry-After", strconv.Itoa(auditVerifyRateLimitWindowSecs))
			respondError(w, r, http.StatusTooManyRequests, "audit verify rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// projectRateLimit enforces per-API-key and per-project rate limits using Redis.
func (s *Server) projectRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.rateLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()

		if isInternalCaller(ctx) {
			next.ServeHTTP(w, r)
			return
		}

		rl := s.resolveRateLimit(ctx, r, projectIDFromContext(ctx), apiKeyIDFromContext(ctx))
		if rl.err != nil {
			slog.Error("rate limit plan lookup failed, failing closed", "error", rl.err)
			respondError(w, r, http.StatusServiceUnavailable, "rate limit plan unavailable")
			return
		}
		if rl.limit == 0 {
			next.ServeHTTP(w, r)
			return
		}

		window := time.Duration(rl.windowSecs) * time.Second
		result, rlErr := s.rateLimiter.AllowStrict(ctx, rl.key, rl.limit, window)
		if rlErr != nil {
			slog.Error("rate limiter error, failing closed", "key", rl.key, "error", rlErr)
			respondError(w, r, http.StatusServiceUnavailable, "rate limit service unavailable")
			return
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

// sensitiveQueryKeywords is the set of substrings that, when present
// in a query parameter name (case-insensitive), trigger value
// redaction in sanitizeQuery. Stored in a map for O(1) substring
// containment checks (still linear over the small keyword set, but no
// allocation per lookup) and unexported so callers cannot mutate it
// at runtime. The list intentionally over-redacts — false positives
// (e.g. an "author" or "design" param) only cost log fidelity, while
// false negatives leak credentials into logs and traces.
var sensitiveQueryKeywords = [...]string{
	"secret",
	"password",
	"token",
	"key",
	"auth",
	"credential",
	"sig",
	"jwt",
	"bearer",
	"hmac",
	"nonce",
	"csrf",
	"state",
	"code_verifier",
	"code_challenge",
	"session",
}

// containsSensitiveKeyword reports whether name contains any of the
// configured credential keywords (case-insensitive). Keyword order is
// irrelevant: containment is commutative across keywords.
func containsSensitiveKeyword(name string) bool {
	lower := strings.ToLower(name)
	for _, kw := range sensitiveQueryKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// sanitizeQuery returns a query parameter string with values for any
// param whose name contains a credential keyword replaced by
// "[REDACTED]". Param names are emitted in sorted order so identical
// inputs produce byte-identical outputs — log/trace consumers can
// dedupe and tests can assert on exact strings.
func sanitizeQuery(params map[string][]string) string {
	if len(params) == 0 {
		return ""
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var sb strings.Builder
	first := true
	for _, k := range keys {
		redact := containsSensitiveKeyword(k)
		for _, v := range params[k] {
			if !first {
				sb.WriteByte('&')
			}
			first = false
			sb.WriteString(k)
			sb.WriteByte('=')
			if redact {
				sb.WriteString("[REDACTED]")
			} else {
				sb.WriteString(v)
			}
		}
	}
	return sb.String()
}
