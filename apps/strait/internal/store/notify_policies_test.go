package store

import (
	"testing"

	"strait/internal/domain"
)

func TestFilterMatchingNotifyPolicies_PreferenceOrder(t *testing.T) {
	t.Parallel()

	project := domain.NotifyPolicyOverride{ID: "p1", ScopeType: domain.NotifyPolicyScopeProject, ScopeKey: "*", Enabled: true}
	category := domain.NotifyPolicyOverride{ID: "c1", ScopeType: domain.NotifyPolicyScopeCategory, ScopeKey: "billing", Enabled: true}
	step := domain.NotifyPolicyOverride{ID: "s1", ScopeType: domain.NotifyPolicyScopeWorkflowStep, ScopeKey: "step_1", Enabled: true}

	matched := filterMatchingNotifyPolicies([]domain.NotifyPolicyOverride{project, category, step}, "step_1", "billing", "email")
	if len(matched) != 3 {
		t.Fatalf("matched count = %d, want 3", len(matched))
	}

	best := matched[0]
	for _, candidate := range matched[1:] {
		if notifyPolicyRank(candidate, "email") < notifyPolicyRank(best, "email") {
			best = candidate
		}
	}
	if best.ID != "s1" {
		t.Fatalf("best match ID = %s, want s1", best.ID)
	}
}

func TestFilterMatchingNotifyPolicies_Adversarial(t *testing.T) {
	t.Parallel()

	candidates := []domain.NotifyPolicyOverride{
		{ID: "disabled", ScopeType: domain.NotifyPolicyScopeProject, ScopeKey: "*", Enabled: false},
		{ID: "other-channel", ScopeType: domain.NotifyPolicyScopeProject, ScopeKey: "*", Channel: "sms", Enabled: true},
		{ID: "bad-scope", ScopeType: "invalid", ScopeKey: "x", Enabled: true},
	}
	matched := filterMatchingNotifyPolicies(candidates, "", "", "email")
	if len(matched) != 0 {
		t.Fatalf("matched count = %d, want 0", len(matched))
	}
}

func FuzzFilterMatchingNotifyPolicies(f *testing.F) {
	f.Add("step_1", "billing", "email")
	f.Add("", "", "")
	f.Add("step_x", "cat_x", "inbox")

	f.Fuzz(func(t *testing.T, stepRunID, categoryKey, channel string) {
		candidates := []domain.NotifyPolicyOverride{
			{ScopeType: domain.NotifyPolicyScopeProject, ScopeKey: "*", Enabled: true},
			{ScopeType: domain.NotifyPolicyScopeCategory, ScopeKey: categoryKey, Enabled: true},
			{ScopeType: domain.NotifyPolicyScopeWorkflowStep, ScopeKey: stepRunID, Enabled: true, Channel: channel},
		}
		_ = filterMatchingNotifyPolicies(candidates, stepRunID, categoryKey, channel)
	})
}
