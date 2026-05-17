package store

import (
	"errors"
	"fmt"
	"testing"
)

// TestErrAPIKeyNotFound_IsExportedSentinel guards the package-level
// presence of the sentinel. Removing it would silently break every caller
// that switched off the legacy err.Error() string compare.
func TestErrAPIKeyNotFound_IsExportedSentinel(t *testing.T) {
	t.Parallel()
	if ErrAPIKeyNotFound == nil {
		t.Fatal("ErrAPIKeyNotFound must be a non-nil sentinel")
	}
	if ErrAPIKeyNotFound.Error() != "api key not found" {
		t.Errorf("sentinel message changed; downstream string-comparing callers may break: %q", ErrAPIKeyNotFound.Error())
	}
}

// TestErrAPIKeyNotFound_MatchableThroughWrapping is the regression guard:
// a caller that wraps the sentinel must still be matchable via errors.Is.
// This is the whole point of the sentinel — string comparison drifts the
// moment any caller reaches for fmt.Errorf("...: %w", err).
func TestErrAPIKeyNotFound_MatchableThroughWrapping(t *testing.T) {
	t.Parallel()
	wrapped := fmt.Errorf("seed pentest: %w", ErrAPIKeyNotFound)
	if !errors.Is(wrapped, ErrAPIKeyNotFound) {
		t.Errorf("wrapped sentinel must match via errors.Is; got %v", wrapped)
	}
}

// TestErrAPIKeyNotFound_DoesNotMatchUnrelatedError keeps the matcher
// strict. A different sentinel must not appear equal to ErrAPIKeyNotFound.
func TestErrAPIKeyNotFound_DoesNotMatchUnrelatedError(t *testing.T) {
	t.Parallel()
	other := errors.New("some unrelated error")
	if errors.Is(other, ErrAPIKeyNotFound) {
		t.Errorf("unrelated error wrongly matched ErrAPIKeyNotFound")
	}
	otherWithSameMessage := errors.New("api key not found")
	if errors.Is(otherWithSameMessage, ErrAPIKeyNotFound) {
		t.Errorf("string-equal but identity-distinct error must NOT match — that defeats the sentinel")
	}
}
