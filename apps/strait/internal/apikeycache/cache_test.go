package apikeycache

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
)

func TestVersionedLoaderNormalizesMissingAPIKeys(t *testing.T) {
	t.Parallel()

	errAPIKeyNotFound := errors.New("api key not found")
	loader := VersionedLoader(func(context.Context, string) (*domain.APIKey, error) {
		return nil, errAPIKeyNotFound
	}, errAPIKeyNotFound)
	got, err := loader(t.Context(), "missing")
	if err != nil {
		t.Fatalf("VersionedLoader() error = %v", err)
	}
	if got.Value != nil || got.Version != 0 {
		t.Fatalf("VersionedLoader() = %+v, want nil@0", got)
	}
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

	if got == key {
		t.Fatal("Sanitize() returned original pointer")
	}
	if got.Scopes[0] != domain.ScopeJobsRead {
		t.Fatalf("sanitized scopes mutated with source: %v", got.Scopes)
	}
	if len(got.RotationWebhookSecret) != 0 {
		t.Fatalf("sanitized key retained rotation secret: %q", got.RotationWebhookSecret)
	}
	if Version(got) != 12 {
		t.Fatalf("Version() = %d, want 12", Version(got))
	}
}
