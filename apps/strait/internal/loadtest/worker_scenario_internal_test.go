//go:build loadtest

package loadtest

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsStreamClosedErr_DoesNotSwallowTransientTransportFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "eof closes cleanly", err: io.EOF, want: true},
		{name: "canceled reconnects while parent context is active", err: status.Error(codes.Canceled, "transport canceled"), want: false},
		{name: "unavailable reconnects", err: status.Error(codes.Unavailable, "server unavailable"), want: false},
		{name: "deadline exceeded reconnects", err: status.Error(codes.DeadlineExceeded, "deadline"), want: false},
		{name: "non grpc error reconnects", err: errors.New("connection reset"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isStreamClosedErr(tt.err))
		})
	}
}
