package api

import (
	"strings"
	"testing"
)

func TestParsePaginationParamsTyped_InvalidCursorMessage(t *testing.T) {
	t.Parallel()

	_, _, err := parsePaginationParamsTyped("10", "not-a-date")
	if err == nil {
		t.Fatal("expected invalid cursor error")
	}
	if !strings.Contains(err.Error(), "cursor must be a valid RFC3339 timestamp") {
		t.Fatalf("error = %q, want typed cursor validation message", err.Error())
	}
}
