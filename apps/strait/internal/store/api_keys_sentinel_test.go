package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrAPIKeyNotFound_IsExportedSentinel guards the package-level
// presence of the sentinel. Removing it would silently break every caller
// that switched off the legacy err.Error() string compare.
func TestErrAPIKeyNotFound_IsExportedSentinel(t *testing.T) {
	t.Parallel()
	require.NotNil(t, ErrAPIKeyNotFound)
	assert.Equal(t,
		"api key not found",

		ErrAPIKeyNotFound.Error())

}

// TestErrAPIKeyNotFound_MatchableThroughWrapping is the regression guard:
// a caller that wraps the sentinel must still be matchable via errors.Is.
// This is the whole point of the sentinel — string comparison drifts the
// moment any caller reaches for fmt.Errorf("...: %w", err).
func TestErrAPIKeyNotFound_MatchableThroughWrapping(t *testing.T) {
	t.Parallel()
	wrapped := fmt.Errorf("seed pentest: %w", ErrAPIKeyNotFound)
	assert.True(t,
		errors.Is(wrapped,

			ErrAPIKeyNotFound,
		))

}

// TestErrAPIKeyNotFound_DoesNotMatchUnrelatedError keeps the matcher
// strict. A different sentinel must not appear equal to ErrAPIKeyNotFound.
func TestErrAPIKeyNotFound_DoesNotMatchUnrelatedError(t *testing.T) {
	t.Parallel()
	other := errors.New("some unrelated error")
	assert.False(t,
		errors.Is(other,
			ErrAPIKeyNotFound,
		))

	otherWithSameMessage := errors.New("api key not found")
	assert.False(t,
		errors.Is(otherWithSameMessage,

			ErrAPIKeyNotFound))

}
