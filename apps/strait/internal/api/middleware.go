package api

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

const ctxProjectIDKey contextKey = "project_id"
const ctxScopesKey contextKey = "scopes"
const ctxAPIKeyIDKey contextKey = "api_key_id"
const ctxActorIDKey contextKey = "actor_id"
const ctxActorTypeKey contextKey = "actor_type" // "user" or "api_key"
const ctxAuthKeyObjKey contextKey = "api_key_obj"

// apiVersion is the current API version returned in response headers.
const apiVersion = "v1"

// sseTokenAuth extracts auth token from ?token= query param for SSE endpoints
// where browsers cannot set custom headers (EventSource API limitation).
func (s *Server) sseTokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Internal-Secret") != "" {
			next.ServeHTTP(w, r)
			return
		}
		token := r.URL.Query().Get("token")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
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

func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer strait_") {
			respondError(w, r, http.StatusUnauthorized, "invalid or missing api key")
			return
		}

		rawKey := strings.TrimPrefix(authHeader, "Bearer ")
		keyHash := hashAPIKey(rawKey)

		apiKey, err := s.store.GetAPIKeyByHash(r.Context(), keyHash)
		if err != nil {
			respondError(w, r, http.StatusUnauthorized, "invalid api key")
			return
		}

		if apiKey.RevokedAt != nil {
			respondError(w, r, http.StatusUnauthorized, "api key has been revoked")
			return
		}

		now := time.Now()
		if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(now) {
			respondError(w, r, http.StatusUnauthorized, "api key has expired")
			return
		}
		if apiKey.GraceExpiresAt != nil && apiKey.GraceExpiresAt.Before(now) {
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
		ctx = context.WithValue(ctx, ctxScopesKey, []string{}) // non-nil => enforce RBAC path in requirePermission
		if projectID := strings.TrimSpace(r.Header.Get("X-Project-Id")); projectID != "" {
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
		secret := r.Header.Get("X-Internal-Secret")
		if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(s.config.InternalSecret)) != 1 {
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
			case "api_key":
				// API keys use scopes directly.
				if domain.HasScope(scopes, permission) {
					next.ServeHTTP(w, r)
					return
				}
				respondError(w, r, http.StatusForbidden, "insufficient permissions: requires "+permission)
				return

			case "user":
				// Users: check role permissions from DB (with cache).
				projectID := projectIDFromContext(ctx)
				actorID := actorFromContext(ctx)
				if projectID == "" || actorID == "" {
					respondError(w, r, http.StatusForbidden, "missing project or actor context")
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

// projectContextMiddleware sets the app.current_project_id session variable
// for RLS policies when the request has a project context.
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

// apiVersionHeader injects X-API-Version into every response.
func apiVersionHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-API-Version", apiVersion)
		next.ServeHTTP(w, r)
	})
}

// projectRateLimit enforces per-API-key and per-project rate limits using Redis.
// Resolution order: API key override → project quota → global config fallback.
func (s *Server) projectRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.rateLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()
		projectID := projectIDFromContext(ctx)
		apiKeyID := apiKeyIDFromContext(ctx)

		var limit int
		var windowSecs int
		var key string

		// 1. Try API key-level rate limit (from context, no DB hit).
		if apiKeyID != "" {
			if apiKey, ok := ctx.Value(ctxAuthKeyObjKey).(*domain.APIKey); ok && apiKey != nil && apiKey.RateLimitRequests > 0 && apiKey.RateLimitWindowSecs > 0 {
				limit = apiKey.RateLimitRequests
				windowSecs = apiKey.RateLimitWindowSecs
				key = "rl:apikey:" + apiKeyID
			}
		}

		// 2. Fall back to project quota rate limit.
		if limit == 0 && projectID != "" {
			quota, err := s.store.GetProjectQuota(ctx, projectID)
			if err == nil && quota != nil && quota.RateLimitRequests > 0 && quota.RateLimitWindowSecs > 0 {
				limit = quota.RateLimitRequests
				windowSecs = quota.RateLimitWindowSecs
				key = "rl:project:" + projectID
			}
		}

		if limit == 0 {
			next.ServeHTTP(w, r)
			return
		}

		window := time.Duration(windowSecs) * time.Second
		result, rlErr := s.rateLimiter.Allow(ctx, key, limit, window)
		if rlErr != nil {
			slog.Warn("rate limiter error, failing open", "key", key, "error", rlErr)
		}
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
		if !result.Allowed {
			w.Header().Set("Retry-After", strconv.Itoa(windowSecs))
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
