package api

import "testing"

func TestValidateNotifyResolvedEmailProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		provider          string
		allowLegacyResend bool
		wantErr           bool
	}{
		{name: "ses allowed", provider: "ses"},
		{name: "empty defaults to ses", provider: ""},
		{name: "resend blocked", provider: "resend", wantErr: true},
		{name: "resend allowed in legacy mode", provider: "resend", allowLegacyResend: true},
		{name: "unsupported blocked", provider: "mailgun", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateNotifyResolvedEmailProvider(tt.provider, tt.allowLegacyResend)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
