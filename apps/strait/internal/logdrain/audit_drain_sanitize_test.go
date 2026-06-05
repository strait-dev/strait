package logdrain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeSIEMEndpoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain-https", "https://siem.example.com/ingest", "https://siem.example.com/ingest"},
		{"strips-query", "https://siem.example.com/ingest?key=supersecret", "https://siem.example.com/ingest"},
		{"strips-userinfo", "https://user:pass@siem.example.com/ingest", "https://siem.example.com/ingest"},
		{"strips-fragment", "https://siem.example.com/ingest#tag", "https://siem.example.com/ingest"},
		{"empty-stays-empty", "", ""},
		{"strips-both", "https://svc:token@host.internal/path?auth=xxx#frag", "https://host.internal/path"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeSIEMEndpoint(tc.in)
			assert.Equal(t,

				tc.want, got)

		})
	}
}
