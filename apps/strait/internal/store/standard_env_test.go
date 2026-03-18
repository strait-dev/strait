package store

import (
	"testing"

	"strait/internal/domain"
)

func TestStandardEnvironmentSlugs(t *testing.T) {
	t.Parallel()
	expected := []string{"development", "staging", "production"}
	if len(domain.StandardEnvironmentSlugs) != 3 {
		t.Fatalf("expected 3 standard slugs, got %d", len(domain.StandardEnvironmentSlugs))
	}
	for i, slug := range domain.StandardEnvironmentSlugs {
		if slug != expected[i] {
			t.Errorf("slug[%d] = %q, want %q", i, slug, expected[i])
		}
	}
}

func TestStandardEnvironmentNames(t *testing.T) {
	t.Parallel()
	for _, slug := range domain.StandardEnvironmentSlugs {
		name, ok := domain.StandardEnvironmentNames[slug]
		if !ok {
			t.Errorf("missing name for slug %q", slug)
		}
		if name == "" {
			t.Errorf("empty name for slug %q", slug)
		}
	}
}

func TestErrStandardEnvironment(t *testing.T) {
	t.Parallel()
	if ErrStandardEnvironment == nil {
		t.Fatal("ErrStandardEnvironment should not be nil")
	}
	if ErrStandardEnvironment.Error() != "cannot modify standard environment" {
		t.Errorf("unexpected error message: %s", ErrStandardEnvironment.Error())
	}
}
