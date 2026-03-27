package api

import (
	"crypto/rsa"
	"fmt"
	"strings"
	"time"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/golang-jwt/jwt/v5"
)

type oidcClaims struct {
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
	Scope string `json:"scope,omitempty"`
	jwt.RegisteredClaims
}

// Scopes returns the parsed scope claim as a string slice, filtered to only
// recognized Strait API scopes. Unrecognized scopes (typos, OIDC-only scopes
// like "openid", or injected values) are silently dropped. Returns nil if no
// recognized scopes are present (meaning no scope restriction).
func (c *oidcClaims) Scopes() []string {
	if c.Scope == "" {
		return nil
	}
	raw := strings.Split(c.Scope, " ")
	var valid []string
	for _, s := range raw {
		if domain.ValidScopes[s] {
			valid = append(valid, s)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	return valid
}

type oidcVerifier struct {
	enabled   bool
	issuer    string
	audience  string
	publicKey *rsa.PublicKey
}

func newOIDCVerifier(cfg *config.Config) (*oidcVerifier, error) {
	v := &oidcVerifier{
		enabled:  cfg.OIDCEnabled,
		issuer:   cfg.OIDCIssuer,
		audience: cfg.OIDCAudience,
	}
	if !v.enabled {
		return v, nil
	}

	pem := strings.TrimSpace(cfg.OIDCPublicKeyPEM)
	if pem == "" {
		return nil, fmt.Errorf("OIDC_PUBLIC_KEY_PEM is required when OIDC is enabled")
	}
	pk, err := jwt.ParseRSAPublicKeyFromPEM([]byte(pem))
	if err != nil {
		return nil, fmt.Errorf("parse oidc public key: %w", err)
	}
	v.publicKey = pk
	return v, nil
}

func (v *oidcVerifier) verify(token string) (*oidcClaims, error) {
	if !v.enabled {
		return nil, fmt.Errorf("oidc verifier disabled")
	}
	claims := &oidcClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return v.publicKey, nil
	},
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return nil, err
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("token subject is required")
	}
	return claims, nil
}
