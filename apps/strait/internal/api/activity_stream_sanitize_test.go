package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStripSSENewlines is the regression guard for SSE frame injection: CR/LF
// bytes in a payload must be removed so a crafted message cannot inject extra
// data:/event: lines into the stream.
func TestStripSSENewlines(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		`{"a":1}`:                   `{"a":1}`,
		"clean":                     "clean",
		"line1\ndata: injected":     "line1data: injected",
		"a\r\nb":                    "ab",
		"\n\nevent: spoof\ndata: x": "event: spoofdata: x",
	}
	for in, want := range cases {
		require.Equal(t, want, string(stripSSENewlines([]byte(in))))
	}
}
