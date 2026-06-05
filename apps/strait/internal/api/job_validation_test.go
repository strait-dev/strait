package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateQueueName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		queue   string
		wantErr bool
	}{
		{name: "empty uses default", queue: "", wantErr: false},
		{name: "single character", queue: "a", wantErr: false},
		{name: "sixty three characters", queue: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantErr: false},
		{name: "hyphen and underscore", queue: "worker-http_1", wantErr: false},
		{name: "too long", queue: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantErr: true},
		{name: "dot", queue: "worker.http", wantErr: true},
		{name: "space", queue: "worker http", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateQueueName(tt.queue)
			require.Equal(t, tt.wantErr, (err != nil))
		})
	}
}
