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

func TestNotifySuppressionReasonRequiresManualOverride_Domain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reason string
		want   bool
	}{
		{name: "ses bounce constant", reason: NotifySuppressionReasonSESBounce, want: true},
		{name: "ses complaint constant", reason: NotifySuppressionReasonSESComplaint, want: true},
		{name: "empty string", reason: "", want: false},
		{name: "unrelated reason", reason: "admin_review", want: false},
		{name: "provider source without known suffix", reason: "provider_callback:ses.delivered", want: false},
		// Exact match only — substring or prefix must not match.
		{name: "prefix bounce only", reason: "provider_callback:ses.bounce.extra", want: false},
		{name: "suffix modified", reason: "provider_callback:ses.bounce_v2", want: false},
		{name: "uppercase", reason: "PROVIDER_CALLBACK:SES.BOUNCE", want: false},
		{name: "whitespace padded", reason: " " + NotifySuppressionReasonSESBounce, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NotifySuppressionReasonRequiresManualOverride(tc.reason); got != tc.want {
				t.Fatalf("NotifySuppressionReasonRequiresManualOverride(%q) = %v, want %v", tc.reason, got, tc.want)
			}
		})
	}
}

func FuzzNotifySuppressionReasonRequiresManualOverride(f *testing.F) {
	f.Add(NotifySuppressionReasonSESBounce)
	f.Add(NotifySuppressionReasonSESComplaint)
	f.Add("")
	f.Add("provider_callback:ses.bounce.extra")
	f.Add("PROVIDER_CALLBACK:SES.BOUNCE")
	f.Add("manual_review")

	f.Fuzz(func(t *testing.T, reason string) {
		result := NotifySuppressionReasonRequiresManualOverride(reason)
		// Invariant: only the two canonical constants may return true.
		if result {
			if reason != NotifySuppressionReasonSESBounce && reason != NotifySuppressionReasonSESComplaint {
				t.Fatalf("NotifySuppressionReasonRequiresManualOverride(%q) = true for non-canonical reason", reason)
			}
		}
	})
}
