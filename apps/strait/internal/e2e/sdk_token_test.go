//go:build integration

package e2e_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func makeE2ERunToken(t *testing.T, runID string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":     "strait:run-token",
		"sub":     runID,
		"attempt": 1,
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(testJWTSigningKey))
	if err != nil {
		t.Fatalf("sign run token: %v", err)
	}
	return signed
}
