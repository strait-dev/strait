package api

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

const ctxProjectIDKey contextKey = "project_id"

func projectIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxProjectIDKey).(string); ok {
		return v
	}
	return ""
}

func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer orc_") {
			respondError(w, http.StatusUnauthorized, "invalid or missing api key")
			return
		}

		rawKey := strings.TrimPrefix(authHeader, "Bearer ")
		keyHash := hashAPIKey(rawKey)

		apiKey, err := s.store.GetAPIKeyByHash(r.Context(), keyHash)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "invalid api key")
			return
		}

		if apiKey.RevokedAt != nil {
			respondError(w, http.StatusUnauthorized, "api key has been revoked")
			return
		}

		if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
			respondError(w, http.StatusUnauthorized, "api key has expired")
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
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) apiKeyOrSecretAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer orc_") {
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
			respondError(w, http.StatusUnauthorized, "invalid or missing internal secret")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("request", //nolint:gosec // structured logging sanitizes values
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes", ww.BytesWritten(),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"request_id", chimw.GetReqID(r.Context()),
		)
	})
}
