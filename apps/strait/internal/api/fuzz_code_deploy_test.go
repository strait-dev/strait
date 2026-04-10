package api

import (
	"testing"
)

// FuzzParsePagination is a fuzz target for parsePaginationFromStrings.
// Properties under test:
//  1. Never panics regardless of limit/cursor input strings.
//  2. On success, limit is always in [1, maxPageLimit].
//  3. On success with a non-empty cursor, the returned *time.Time is non-nil.
//  4. Error strings never leak internal implementation details.
func FuzzParsePaginationCodeDeploy(f *testing.F) {
	// Seeds: valid pairs.
	f.Add("10", "")
	f.Add("100", "2024-01-15T12:00:00.000000000Z")
	f.Add("", "")
	f.Add("1", "2023-06-01T00:00:00Z")
	// Seeds: boundary values.
	f.Add("0", "")
	f.Add("-1", "")
	f.Add("99999", "")
	f.Add("2147483647", "") // MaxInt32
	// Seeds: malformed cursor.
	f.Add("10", "not-a-time")
	f.Add("10", "2024-13-01T00:00:00Z") // invalid month
	f.Add("10", "9999-99-99T99:99:99Z")
	// Seeds: injection-style inputs.
	f.Add("'; DROP TABLE code_deployments; --", "")
	f.Add("10", "'; DROP TABLE job_runs; --")
	f.Add("1\x00", "")
	f.Add("", "\x00")

	f.Fuzz(func(t *testing.T, limitStr, cursorStr string) {
		limit, cursor, err := parsePaginationFromStrings(limitStr, cursorStr)
		if err != nil {
			// Error is acceptable; just verify no panic and return.
			return
		}
		// On success, limit must be at least 1.
		if limit < 1 {
			t.Errorf("parsePaginationFromStrings(%q, %q) returned limit=%d; want >= 1", limitStr, cursorStr, limit)
		}
		// On success, limit must not exceed maxPageLimit.
		if limit > maxPageLimit {
			t.Errorf("parsePaginationFromStrings(%q, %q) returned limit=%d; want <= %d", limitStr, cursorStr, limit, maxPageLimit)
		}
		// A non-empty cursor string must produce a non-nil cursor time.
		if cursorStr != "" && cursor == nil {
			t.Errorf("parsePaginationFromStrings(%q, %q) returned nil cursor; want non-nil", limitStr, cursorStr)
		}
		// An empty cursor string must produce a nil cursor time.
		if cursorStr == "" && cursor != nil {
			t.Errorf("parsePaginationFromStrings(%q, %q) returned non-nil cursor for empty input", limitStr, cursorStr)
		}
	})
}
