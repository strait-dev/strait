package domain

import "testing"

func TestValidateScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		scopes  []string
		wantErr bool
	}{
		{"empty is valid", []string{}, false},
		{"wildcard", []string{"*"}, false},
		{"single valid", []string{"jobs:read"}, false},
		{"multiple valid", []string{"jobs:read", "runs:write", "workflows:trigger"}, false},
		{"unknown scope", []string{"foo:bar"}, true},
		{"mix valid and invalid", []string{"jobs:read", "invalid"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateScopes(tt.scopes)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateScopes(%v) error = %v, wantErr %v", tt.scopes, err, tt.wantErr)
			}
		})
	}
}

func TestHasScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		scopes   []string
		required string
		want     bool
	}{
		{"empty scopes allows all", []string{}, "jobs:read", true},
		{"wildcard allows all", []string{"*"}, "jobs:write", true},
		{"exact match", []string{"jobs:read"}, "jobs:read", true},
		{"no match", []string{"jobs:read"}, "jobs:write", false},
		{"multiple with match", []string{"jobs:read", "runs:read"}, "runs:read", true},
		{"multiple without match", []string{"jobs:read", "runs:read"}, "workflows:write", false},
		{"wildcard among others", []string{"jobs:read", "*"}, "anything", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := HasScope(tt.scopes, tt.required)
			if got != tt.want {
				t.Errorf("HasScope(%v, %q) = %v, want %v", tt.scopes, tt.required, got, tt.want)
			}
		})
	}
}
