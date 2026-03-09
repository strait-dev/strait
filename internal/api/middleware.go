package api

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"orchestrator/internal/domain"

	chimw "github.com/go-chi/chi/v5/middleware"
)

const ctxProjectIDKey contextKey = "project_id"
const ctxScopesKey contextKey = "scopes"
const ctxAPIKeyIDKey contextKey = "api_key_id"
const ctxActorIDKey contextKey = "actor_id"
const ctxActorTypeKey contextKey = "actor_type" // "user" or "api_key"

// apiVersion is the current API version returned in response headers.
const apiVersion = "v1"

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

		if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
			respondError(w, r, http.StatusUnauthorized, "api key has expired")
			return
		}

		touchCtx := context.WithoutCancel(r.Context())
		go func() {
			ctx, cancel := context.WithTimeout(touchCtx, 2*time.Second)
			defer cancel()
			if err := s.store.TouchAPIKeyLastUsed(ctx, apiKey.ID); err != nil {
				slog.Error("failed to touch api key last used", "key_id", apiKey.ID, "error", err)
			}
		}()

		ctx := context.WithValue(r.Context(), ctxProjectIDKey, apiKey.ProjectID)
		ctx = context.WithValue(ctx, ctxScopesKey, apiKey.Scopes)
		ctx = context.WithValue(ctx, ctxAPIKeyIDKey, apiKey.ID)

		// Extract actor identity from trusted app headers.
		// If X-Actor-Id is present, the request is on behalf of a user.
		// Otherwise, the API key itself is the actor.
		if actorID := r.Header.Get("X-Actor-Id"); actorID != "" {
			ctx = context.WithValue(ctx, ctxActorIDKey, actorID)
			ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

			// Lazy-sync actor profile if store supports it
			if s.actorSyncer != nil {
				actorEmail := r.Header.Get("X-Actor-Email")
				actorName := r.Header.Get("X-Actor-Name")
				syncCtx := context.WithoutCancel(ctx)
				go func() {
					syncCtx, cancel := context.WithTimeout(syncCtx, 2*time.Second)
					defer cancel()
					if err := s.actorSyncer.UpsertKnownActor(syncCtx, actorID, actorEmail, actorName); err != nil {
						slog.Warn("failed to sync actor", "actor_id", actorID, "error", err)
					}
				}()
			}
		} else {
			ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:"+apiKey.ID)
			ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
		}

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

		s.internalSecretAuth(next).ServeHTTP(w, r)
	})
}

func (s *Server) internalSecretAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := r.Header.Get("X-Internal-Secret")
		if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(s.config.InternalSecret)) != 1 {
			respondError(w, r, http.StatusUnauthorized, "invalid or missing internal secret")
			return
		}
		next.ServeHTTP(w, r)
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

// requireScope returns a middleware that checks whether the authenticated
// caller has the given scope. Empty scopes or wildcard are treated as full
// access for backwards compatibility with pre-existing API keys.
// Requests authenticated via internal secret (no scopes in context) are always allowed.
func requireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scopes := scopesFromContext(r.Context())
			// nil scopes means internal secret auth (no API key) — allow through
			if scopes == nil {
				next.ServeHTTP(w, r)
				return
			}
			if !domain.HasScope(scopes, scope) {
				respondError(w, r, http.StatusForbidden, "insufficient permissions: requires "+scope)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requirePermission returns a middleware that checks authorization based on
// the actor type. For API keys, it checks scopes. For users, it loads their
// role permissions from the database.
func (s *Server) requirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			actorType, _ := ctx.Value(ctxActorTypeKey).(string)

			switch actorType {
			case "api_key":
				// API keys use scopes directly
				scopes := scopesFromContext(ctx)
				if scopes == nil || domain.HasScope(scopes, permission) {
					next.ServeHTTP(w, r)
					return
				}
				respondError(w, r, http.StatusForbidden, "insufficient permissions: requires "+permission)
				return

			case "user":
				// Users: check role permissions from DB
				projectID := projectIDFromContext(ctx)
				actorID := actorFromContext(ctx)
				if projectID == "" || actorID == "" {
					respondError(w, r, http.StatusForbidden, "missing project or actor context")
					return
				}
				perms, err := s.store.GetUserPermissions(ctx, projectID, actorID)
				if err != nil {
					respondError(w, r, http.StatusInternalServerError, "failed to load permissions")
					return
				}
				if perms == nil {
					respondError(w, r, http.StatusForbidden, "no role assigned in this project")
					return
				}
				if domain.HasScope(perms, permission) {
					next.ServeHTTP(w, r)
					return
				}
				respondError(w, r, http.StatusForbidden, "insufficient permissions: requires "+permission)
				return

			default:
				// Internal secret or unknown — allow (internal secret has no scopes)
				next.ServeHTTP(w, r)
			}
		})
	}
}

// apiVersionHeader injects X-API-Version into every response.
func apiVersionHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-API-Version", apiVersion)
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
