package apikeycache

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ttl  time.Duration
		want time.Duration
	}{
		{name: "positive ttl uses third", ttl: 90 * time.Second, want: 30 * time.Second},
		{name: "small positive ttl keeps minimum nanosecond", ttl: time.Nanosecond, want: time.Nanosecond},
		{name: "zero ttl stays zero", ttl: 0, want: 0},
		{name: "negative ttl stays negative", ttl: -3 * time.Second, want: -3 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, RefreshAfter(tt.ttl))
		})
	}
}

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

func TestVersionedLoaderReturnsUnexpectedErrors(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	loader := VersionedLoader(func(context.Context, string) (*domain.APIKey, error) {
		return nil, errBoom
	}, errors.New("api key not found"))

	got, err := loader(t.Context(), "broken")
	require.ErrorIs(t, err, errBoom)
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
