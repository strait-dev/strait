package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAcceptsGzip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "empty", header: "", want: false},
		{name: "gzip", header: "gzip", want: true},
		{name: "gzip quality", header: "gzip;q=0.8", want: true},
		{name: "case insensitive", header: "GZip", want: true},
		{name: "late match", header: "br;q=1, deflate;q=0.8, gzip;q=0.6", want: true},
		{name: "no match", header: "br;q=1, deflate;q=0.8", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, acceptsGzip(tt.header))
		})
	}
}

func BenchmarkAcceptsGzip(b *testing.B) {
	benchmarks := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "empty", header: "", want: false},
		{name: "gzip", header: "gzip", want: true},
		{name: "gzip_quality", header: "gzip;q=0.8", want: true},
		{name: "late_match", header: "br;q=1, deflate;q=0.8, gzip;q=0.6", want: true},
		{name: "no_match", header: "br;q=1, deflate;q=0.8", want: false},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if got := acceptsGzip(bm.header); got != bm.want {
					b.Fatalf("acceptsGzip() = %v, want %v", got, bm.want)
				}
			}
		})
	}
}
