package store

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandardEnvironmentSlugs(t *testing.T) {
	t.Parallel()
	expected := []string{"development", "staging", "production"}
	require.Len(t,
		domain.
			StandardEnvironmentSlugs,

		3)

	for i, slug := range domain.StandardEnvironmentSlugs {
		assert.Equal(t,

			expected[i], slug)

	}
}

func TestStandardEnvironmentNames(t *testing.T) {
	t.Parallel()
	for _, slug := range domain.StandardEnvironmentSlugs {
		name, ok := domain.StandardEnvironmentNames[slug]
		assert.True(t,
			ok,
		)
		assert.NotEqual(
			t, "", name)

	}
}

func TestErrStandardEnvironment(t *testing.T) {
	t.Parallel()
	require.NotNil(
		t,
		ErrStandardEnvironment)
	assert.Equal(t,

		"cannot modify standard environment",

		ErrStandardEnvironment.Error())

}
