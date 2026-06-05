package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func FuzzParsePagination_Additional(f *testing.F) {
	f.Add("10", "")
	f.Add("", "")
	f.Add("abc", "")
	f.Add("-1", "")
	f.Add("0", "")
	f.Add("999999", "")
	f.Add("10", "2024-01-01T00:00:00Z")
	f.Add("10", "not-a-time")
	f.Add("1", "2024-01-01T00:00:00.123456789Z")
	f.Add("50", "2024-12-31T23:59:59Z")
	f.Add("10", "\x00")
	f.Add("10", "../../../etc/passwd")
	f.Add("10", "2024-01-01T00:00:00+05:30")
	f.Add(strings.Repeat("9", 100), "")
	f.Add("10", strings.Repeat("a", 1000))
	f.Add("2147483647", "")  // max int32
	f.Add("-2147483648", "") // min int32

	f.Fuzz(func(t *testing.T, limitStr, cursorStr string) {
		// parsePaginationFromStrings must never panic regardless of input.
		limit, cursor, err := parsePaginationFromStrings(limitStr, cursorStr)

		if err == nil {
			assert.False(
				t, limit <=
					0,
			)

			// When successful, limit must be positive and bounded.

			// cursor can be nil (no cursor provided) or a valid time pointer.
			_ = cursor
		}
	})
}

func FuzzValidateIDFormat_Additional(f *testing.F) {
	f.Add("abc123def456")
	f.Add("")
	f.Add("../../etc/passwd")
	f.Add("../secret")
	f.Add("job/123")
	f.Add("job\x00-123")
	f.Add(strings.Repeat("a", 300))
	f.Add(strings.Repeat("a", maxIDLength))
	f.Add(strings.Repeat("a", maxIDLength+1))
	f.Add("valid-id-with-dashes")
	f.Add("\t\n\r")
	f.Add("id with spaces")
	f.Add("\xff\xfe")
	f.Add("env-" + strings.Repeat("\u0000", 10))

	f.Fuzz(func(t *testing.T, id string) {
		// validateIDFormat must never panic regardless of input.
		err := validateIDFormat(id)
		require.False(t, id == "" &&
			err == nil)
		require.False(t, strings.ContainsRune(id, '\x00') && err == nil)
		require.False(t, len(id) >

			maxIDLength && err == nil)
		require.False(t, strings.Contains(id, "..") && err == nil)

		// Invariant: empty IDs are always rejected.

		// Invariant: IDs with null bytes are rejected.

		// Invariant: IDs over max length are rejected.

		// Invariant: path traversal patterns are rejected.

	})
}

func FuzzValidateJobSlug_Additional(f *testing.F) {
	f.Add("my-job-slug")
	f.Add("")
	f.Add(strings.Repeat("x", 200))
	f.Add("slug/with/slashes")
	f.Add("\t\n")
	f.Add("UPPER-CASE")
	f.Add("slug_with_underscores")
	f.Add("slug with spaces")
	f.Add("slug\x00null")
	f.Add(strings.Repeat("a", 255))
	f.Add(strings.Repeat("a", 256))
	f.Add("a")
	f.Add("-starts-with-dash")
	f.Add("ends-with-dash-")
	f.Add("has--double--dashes")

	f.Fuzz(func(t *testing.T, slug string) {
		// validateJobSlug must never panic regardless of input.
		_ = validateJobSlug(slug)
	})
}
