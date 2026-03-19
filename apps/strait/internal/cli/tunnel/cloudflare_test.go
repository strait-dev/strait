package tunnel

import "testing"

func TestParseTunnelURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "typical cloudflared output",
			input: `2024-01-15T10:00:00Z INF +-----------------------------------------------------------+`,
			want:  "",
		},
		{
			name:  "line with tunnel URL",
			input: `2024-01-15T10:00:01Z INF | https://random-slug-here.trycloudflare.com |`,
			want:  "https://random-slug-here.trycloudflare.com",
		},
		{
			name:  "no match",
			input: "some random log output",
			want:  "",
		},
		{
			name:  "URL with numbers",
			input: `INF https://abc-123-def.trycloudflare.com ready`,
			want:  "https://abc-123-def.trycloudflare.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ParseTunnelURL(tc.input)
			if got != tc.want {
				t.Fatalf("ParseTunnelURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
