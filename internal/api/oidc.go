package api

import (
	"crypto/rsa"
	"fmt"
	"strings"

	"strait/internal/config"

	"github.com/golang-jwt/jwt/v5"
)

type oidcClaims struct {
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
	jwt.RegisteredClaims
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
		jwt.WithLeeway(30),
	)
	if err != nil {
		return nil, err
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("token subject is required")
	}
	return claims, nil
}
