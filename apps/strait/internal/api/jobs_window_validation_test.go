package api

import (
	"testing"
	"time"

	"strait/internal/config"
)

func TestValidateWindowsAgainstRetention(t *testing.T) {
	t.Parallel()

	srv := &Server{
		config: &config.Config{
			RunRetentionShort: 720 * time.Hour, // 30 days = 2592000 seconds
		},
	}
	maxSecs := int(srv.config.RunRetentionShort.Seconds())

	tests := []struct {
		name    string
		rlw     int
		dw      int
		wantErr bool
	}{
		{"both zero", 0, 0, false},
		{"rate_limit within retention", 3600, 0, false},
		{"dedup within retention", 0, 3600, false},
		{"rate_limit at max", maxSecs, 0, false},
		{"dedup at max", 0, maxSecs, false},
		{"rate_limit exceeds retention", maxSecs + 1, 0, true},
		{"dedup exceeds retention", 0, maxSecs + 1, true},
		{"both exceed retention", maxSecs + 1, maxSecs + 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := srv.validateWindowsAgainstRetention(tt.rlw, tt.dw)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWindowsAgainstRetention(%d, %d) error = %v, wantErr %v", tt.rlw, tt.dw, err, tt.wantErr)
			}
		})
	}
}

func TestValidateWindowsAgainstRetention_NilConfig(t *testing.T) {
	t.Parallel()

	srv := &Server{config: nil}
	if err := srv.validateWindowsAgainstRetention(999999999, 999999999); err != nil {
		t.Errorf("nil config should skip validation, got error: %v", err)
	}
}
