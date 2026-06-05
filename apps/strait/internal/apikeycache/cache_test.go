package apikeycache

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionedLoaderNormalizesMissingAPIKeys(t *testing.T) {
	t.Parallel()

	errAPIKeyNotFound := errors.New("api key not found")
	loader := VersionedLoader(func(context.Context, string) (*domain.APIKey, error) {
		return nil, errAPIKeyNotFound
	}, errAPIKeyNotFound)
	got, err := loader(t.Context(), "missing")
	require.NoError(t, err)
	assert.Nil(t, got.Value)
	assert.Zero(t, got.Version)
}

func TestSanitizeClonesAndRemovesRotationSecret(t *testing.T) {
	t.Parallel()

	key := &domain.APIKey{
		Scopes:                []string{domain.ScopeJobsRead},
		RotationWebhookSecret: []byte("secret"),
		CacheVersion:          12,
	}
	got := Sanitize(key)
	key.Scopes[0] = domain.ScopeJobsWrite

	assert.NotSame(t, key, got)
	assert.Equal(t, domain.ScopeJobsRead, got.Scopes[0])
	assert.Empty(t, got.RotationWebhookSecret)
	assert.Equal(t, int64(12), Version(got))
}
