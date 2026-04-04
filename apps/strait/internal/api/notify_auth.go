package api

import (
	"context"
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
	now := time.Now().UTC()
	claims := notifySubscriberClaims{
		SubscriberID: subscriberID,
		ProjectID:    projectID,
		TenantID:     tenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiresIn)),
			Subject:   subscriberID,
		},
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(s.config.JWTSigningKey))
}

func (s *Server) parseNotifySubscriberToken(raw string) (*notifySubscriberClaims, error) {
	claims := &notifySubscriberClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(_ *jwt.Token) (any, error) {
		return []byte(s.config.JWTSigningKey), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}
	if claims.SubscriberID == "" {
		claims.SubscriberID = claims.Subject
	}
	return claims, nil
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
