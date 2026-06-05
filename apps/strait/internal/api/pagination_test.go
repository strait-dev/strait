package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePaginationParamsTyped_InvalidCursorMessage(t *testing.T) {
	t.Parallel()

	_, _, err := parsePaginationParamsTyped("10", "not-a-date")
	require.Error(t, err)
	require.Contains(
		t, err.Error(), "cursor must be a valid RFC3339 timestamp")
}
