package domain

import "testing"

func TestVersionPolicy_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		policy VersionPolicy
		want   bool
	}{
		{VersionPolicyPin, true},
		{VersionPolicyLatest, true},
		{VersionPolicyMinor, true},
		{VersionPolicy(""), false},
		{VersionPolicy("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.policy), func(t *testing.T) {
			t.Parallel()
			if got := tt.policy.IsValid(); got != tt.want {
				t.Errorf("VersionPolicy(%q).IsValid() = %v, want %v", tt.policy, got, tt.want)
			}
		})
	}
}
