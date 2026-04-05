package domain

import "testing"

func TestNotifyPolicyOverrideValidate(t *testing.T) {
	t.Parallel()

	retryFive := 5
	retryBase := 120
	retryMax := 60

	tests := []struct {
		name     string
		override *NotifyPolicyOverride
		wantErr  bool
	}{
		{
			name: "valid project scope",
			override: &NotifyPolicyOverride{
				ProjectID:        "proj_1",
				ScopeType:        NotifyPolicyScopeProject,
				ScopeKey:         "*",
				DigestPolicy:     "hourly",
				RetryMaxAttempts: &retryFive,
			},
			wantErr: false,
		},
		{
			name: "invalid scope type",
			override: &NotifyPolicyOverride{
				ProjectID: "proj_1",
				ScopeType: "team",
				ScopeKey:  "x",
			},
			wantErr: true,
		},
		{
			name: "invalid digest policy",
			override: &NotifyPolicyOverride{
				ProjectID:    "proj_1",
				ScopeType:    NotifyPolicyScopeCategory,
				ScopeKey:     "cat_1",
				DigestPolicy: "weekly",
			},
			wantErr: true,
		},
		{
			name: "invalid retry bound",
			override: &NotifyPolicyOverride{
				ProjectID:          "proj_1",
				ScopeType:          NotifyPolicyScopeProject,
				ScopeKey:           "*",
				RetryBaseDelaySecs: &retryBase,
				RetryMaxDelaySecs:  &retryMax,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.override.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func FuzzNotifyPolicyOverrideValidate(f *testing.F) {
	f.Add("proj_1", NotifyPolicyScopeProject, "*", "instant")
	f.Add("proj_1", "bad_scope", "k", "hourly")
	f.Add("", NotifyPolicyScopeCategory, "cat", "daily")

	f.Fuzz(func(t *testing.T, projectID, scopeType, scopeKey, digestPolicy string) {
		override := &NotifyPolicyOverride{
			ProjectID:    projectID,
			ScopeType:    scopeType,
			ScopeKey:     scopeKey,
			DigestPolicy: digestPolicy,
		}
		_ = override.Validate()
	})
}
