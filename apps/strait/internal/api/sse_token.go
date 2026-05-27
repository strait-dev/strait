package api

import (
	"context"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/golang-jwt/jwt/v5"
)

const (
	sseTokenTTL    = 5 * time.Minute
	sseTokenIssuer = "strait:sse"
)

// SSETokenClaims extends standard JWT claims with the project, environment, and scopes.
type SSETokenClaims struct {
	jwt.RegisteredClaims
	ProjectID     string   `json:"pid"`
	EnvironmentID string   `json:"eid,omitempty"`
	Scopes        []string `json:"scp,omitempty"`
}

type CreateSSETokenInput struct{}
type CreateSSETokenOutput struct {
	Body struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
}

// handleCreateSSEToken issues a short-lived JWT (5 minutes) for use as a
// query-param token in SSE endpoints. This avoids exposing the long-lived
// API key in URLs.
func (s *Server) handleCreateSSEToken(ctx context.Context, _ *CreateSSETokenInput) (*CreateSSETokenOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	scopes, err := s.scopesForSSEToken(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	expiresAt := now.Add(sseTokenTTL)

	claims := SSETokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sseTokenIssuer,
			Subject:   projectID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		ProjectID:     projectID,
		EnvironmentID: environmentIDFromContext(ctx),
		Scopes:        scopes,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.config.JWTSigningKey))
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create SSE token")
	}

	s.emitAuditEvent(ctx, domain.AuditActionSSETokenCreated, "sse_token", projectID, map[string]any{
		"expires_at":     expiresAt,
		"scope_count":    len(scopes),
		"environment_id": claims.EnvironmentID,
	})

	out := &CreateSSETokenOutput{}
	out.Body.Token = tokenString
	out.Body.ExpiresAt = expiresAt
	return out, nil
}

func (s *Server) scopesForSSEToken(ctx context.Context) ([]string, error) {
	scopes := scopesFromContext(ctx)
	if actorTypeFromContext(ctx) != "user" {
		return scopes, nil
	}

	projectID := projectIDFromContext(ctx)
	actorID := actorFromContext(ctx)
	if projectID == "" || actorID == "" {
		return nil, huma.Error403Forbidden("missing project or actor context")
	}
	if s.store == nil {
		return nil, huma.Error503ServiceUnavailable("service unavailable")
	}

	perms, cached := s.permCache.Get(projectID, actorID)
	if !cached {
		var version int64
		var err error
		perms, version, err = s.loadUserPermissionsForCache(ctx, projectID, actorID)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to load permissions")
		}
		if perms != nil {
			s.permCache.SetWithVersion(projectID, actorID, perms, version)
		}
	}
	if ctx.Value(ctxOIDCScopeClaimPresentKey) == true {
		perms = intersectScopes(perms, scopes)
	}
	if len(perms) == 0 {
		return nil, huma.Error403Forbidden("insufficient permissions")
	}
	return perms, nil
}

func intersectScopes(perms, scopes []string) []string {
	if domain.HasScopeStrict(perms, domain.ScopeAll) {
		return dedupeScopes(scopes)
	}
	if domain.HasScopeStrict(scopes, domain.ScopeAll) {
		return dedupeScopes(perms)
	}

	allowed := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		allowed[scope] = struct{}{}
	}

	out := make([]string, 0, min(len(perms), len(scopes)))
	seen := make(map[string]struct{}, len(perms))
	for _, scope := range perms {
		if _, ok := allowed[scope]; !ok {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

func dedupeScopes(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

// parseSSEToken validates a short-lived SSE JWT and returns the claims.
// Returns nil if the token is invalid or not an SSE token.
func (s *Server) parseSSEToken(tokenString string) *SSETokenClaims {
	claims := &SSETokenClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(s.config.JWTSigningKey), nil
	})
	if err != nil || !token.Valid {
		return nil
	}
	if claims.Issuer != sseTokenIssuer {
		return nil
	}
	return claims
}
