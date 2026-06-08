package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSDKCapabilitiesHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   SDKCapabilities
		want string
	}{
		{name: "none", in: SDKCapabilities{}, want: "none"},
		{name: "progress", in: SDKCapabilities{Progress: true}, want: "progress"},
		{name: "checkpoint", in: SDKCapabilities{Checkpoint: true}, want: "checkpoint"},
		{name: "both", in: SDKCapabilities{Progress: true, Checkpoint: true}, want: "progress,checkpoint"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, sdkCapabilitiesHeader(tt.in))
		})
	}
}

func BenchmarkSDKCapabilitiesHeader(b *testing.B) {
	cases := []SDKCapabilities{
		{},
		{Progress: true},
		{Checkpoint: true},
		{Progress: true, Checkpoint: true},
	}

	b.ReportAllocs()
	for b.Loop() {
		for _, c := range cases {
			if sdkCapabilitiesHeader(c) == "" {
				b.Fatal("empty capabilities header")
			}
		}
	}
}

func BenchmarkResolveSDKCapabilities(b *testing.B) {
	cases := []string{"1.0", "2.0", "2.1.3", "10.0", "abc", " 2.0 "}

	b.ReportAllocs()
	for b.Loop() {
		for _, version := range cases {
			_ = resolveSDKCapabilities(version)
		}
	}
}
