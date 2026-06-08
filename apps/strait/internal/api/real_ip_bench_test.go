package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPFromRequestUsesStampedContextAndFallsBack(t *testing.T) {
	internalProxies := parseTrustedProxies([]string{"10.0.0.0/8"})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:443"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.8")

	if got := clientIPFromRequest(r, internalProxies); got != "198.51.100.7" {
		t.Fatalf("clientIPFromRequest() fallback = %q, want %q", got, "198.51.100.7")
	}

	r = r.WithContext(context.WithValue(r.Context(), ctxRemoteIPKey, "203.0.113.9"))
	if got := clientIPFromRequest(r, internalProxies); got != "203.0.113.9" {
		t.Fatalf("clientIPFromRequest() cached = %q, want %q", got, "203.0.113.9")
	}
}

func BenchmarkRealIP(b *testing.B) {
	internalProxies := parseTrustedProxies([]string{"10.0.0.0/8"})

	benchmarks := []struct {
		name    string
		xff     string
		addr    string
		trusted bool
		want    string
	}{
		{
			name: "direct_no_trusted_proxy",
			addr: "203.0.113.9:443",
			want: "203.0.113.9",
		},
		{
			name:    "trusted_single_hop",
			xff:     "198.51.100.7",
			addr:    "10.0.0.5:443",
			trusted: true,
			want:    "198.51.100.7",
		},
		{
			name:    "trusted_multi_hop",
			xff:     "198.51.100.7, 10.0.0.7, 10.0.0.8",
			addr:    "10.0.0.5:443",
			trusted: true,
			want:    "198.51.100.7",
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = bm.addr
			if bm.xff != "" {
				r.Header.Set("X-Forwarded-For", bm.xff)
			}
			var trusted []net.IPNet
			if bm.trusted {
				trusted = internalProxies
			}

			b.ReportAllocs()
			for b.Loop() {
				if got := realIP(r, trusted); got != bm.want {
					b.Fatalf("realIP() = %q, want %q", got, bm.want)
				}
			}
		})
	}
}

func BenchmarkClientIPFromRequest(b *testing.B) {
	internalProxies := parseTrustedProxies([]string{"10.0.0.0/8"})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:443"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.7, 10.0.0.8")
	r = r.WithContext(context.WithValue(r.Context(), ctxRemoteIPKey, "198.51.100.7"))

	b.ReportAllocs()
	for b.Loop() {
		if got := clientIPFromRequest(r, internalProxies); got != "198.51.100.7" {
			b.Fatalf("clientIPFromRequest() = %q, want %q", got, "198.51.100.7")
		}
	}
}
