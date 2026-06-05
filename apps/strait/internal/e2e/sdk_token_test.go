//go:build integration

package e2e_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

	return signed
}

func activateE2ERun(t *testing.T, runID string) {
	t.Helper()

	ctx := context.Background()
	require.NoError(t, testStore.
		UpdateRunStatus(ctx, runID,
			domain.
				StatusQueued, domain.StatusDequeued,

			map[string]any{"started_at": time.Now().UTC()}))
	require.NoError(t, testStore.
		UpdateRunStatus(ctx, runID,
			domain.
				StatusDequeued, domain.StatusExecuting,

			map[string]any{}),
	)

}
