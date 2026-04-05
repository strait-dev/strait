package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"strait/internal/domain"

	"github.com/golang-jwt/jwt/v5"
)

type notifySubscriberClaims struct {
	SubscriberID string `json:"sub"`
	ProjectID    string `json:"pid"`
	TenantID     string `json:"tid,omitempty"`
	jwt.RegisteredClaims
}

func (s *Server) createNotifySubscriberToken(subscriberID, projectID, tenantID string, expiresIn time.Duration) (string, error) {
	if expiresIn <= 0 {
		expiresIn = 24 * time.Hour
	}
	issuer := strings.TrimSpace(s.config.NotifySubscriberTokenIssuer)
	if issuer == "" {
		issuer = "strait-notify"
	}
	audience := strings.TrimSpace(s.config.NotifySubscriberTokenAudience)
	if audience == "" {
		audience = "strait-notify-subscriber"
	}

	now := time.Now().UTC()
	jti, err := generateSecureToken(16)
	if err != nil {
		return "", err
	}

	claims := notifySubscriberClaims{
		SubscriberID: subscriberID,
		ProjectID:    projectID,
		TenantID:     tenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiresIn)),
			Subject:   subscriberID,
			ID:        jti,
		},
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(s.config.JWTSigningKey))
}

func (s *Server) parseNotifySubscriberToken(raw string) (*notifySubscriberClaims, error) {
	issuer := strings.TrimSpace(s.config.NotifySubscriberTokenIssuer)
	if issuer == "" {
		issuer = "strait-notify"
	}
	audience := strings.TrimSpace(s.config.NotifySubscriberTokenAudience)
	if audience == "" {
		audience = "strait-notify-subscriber"
	}

	claims := &notifySubscriberClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(_ *jwt.Token) (any, error) {
		return []byte(s.config.JWTSigningKey), nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
	)
	if err != nil {
		return nil, err
	}
	if claims.SubscriberID == "" {
		claims.SubscriberID = claims.Subject
	}
	if claims.SubscriberID == "" || claims.ProjectID == "" {
		return nil, errors.New("invalid subscriber token claims")
	}
	if claims.Subject != "" && claims.Subject != claims.SubscriberID {
		return nil, errors.New("subscriber token subject mismatch")
	}
	return claims, nil
}

func generateSecureToken(n int) (string, error) {
	if n <= 0 {
		n = 16
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *Server) notifySubscriberAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			respondError(w, r, http.StatusUnauthorized, "missing subscriber token")
			return
		}

		rawToken := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := s.parseNotifySubscriberToken(rawToken)
		if err != nil {
			respondError(w, r, http.StatusUnauthorized, "invalid subscriber token")
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, ctxProjectIDKey, claims.ProjectID)
		ctx = context.WithValue(ctx, ctxNotifyRecipientTypeKey, domain.NotifyRecipientTypeSubscriber)
		ctx = context.WithValue(ctx, ctxNotifyRecipientIDKey, claims.SubscriberID)
		ctx = context.WithValue(ctx, ctxNotifyTenantIDKey, claims.TenantID)
		ctx = context.WithValue(ctx, ctxActorIDKey, "subscriber:"+claims.SubscriberID)
		ctx = context.WithValue(ctx, ctxActorTypeKey, "subscriber")
		ctx = context.WithValue(ctx, ctxScopesKey, []string{})

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
