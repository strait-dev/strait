package domain

import "fmt"

func (p *NotifyPolicyOverride) Validate() error {
	if p == nil {
		return fmt.Errorf("notify policy override is nil")
	}
	if p.ProjectID == "" {
		return fmt.Errorf("notify policy override project_id is required")
	}
	switch p.ScopeType {
	case NotifyPolicyScopeProject, NotifyPolicyScopeCategory, NotifyPolicyScopeWorkflowStep:
		// valid
	default:
		return fmt.Errorf("notify policy override scope_type %q is invalid", p.ScopeType)
	}
	if p.ScopeKey == "" {
		return fmt.Errorf("notify policy override scope_key is required")
	}
	if p.DigestPolicy != "" {
		switch p.DigestPolicy {
		case "instant", "hourly", "daily":
			// valid
		default:
			return fmt.Errorf("notify policy override digest_policy %q is invalid", p.DigestPolicy)
		}
	}

	if p.RetryMaxAttempts != nil && *p.RetryMaxAttempts <= 0 {
		return fmt.Errorf("notify policy override retry_max_attempts must be > 0")
	}
	if p.RetryBaseDelaySecs != nil && *p.RetryBaseDelaySecs <= 0 {
		return fmt.Errorf("notify policy override retry_base_delay_secs must be > 0")
	}
	if p.RetryMaxDelaySecs != nil && *p.RetryMaxDelaySecs <= 0 {
		return fmt.Errorf("notify policy override retry_max_delay_secs must be > 0")
	}
	if p.RetryBaseDelaySecs != nil && p.RetryMaxDelaySecs != nil && *p.RetryMaxDelaySecs < *p.RetryBaseDelaySecs {
		return fmt.Errorf("notify policy override retry_max_delay_secs must be >= retry_base_delay_secs")
	}
	if p.EscalationTiers != nil && *p.EscalationTiers <= 0 {
		return fmt.Errorf("notify policy override escalation_tiers must be > 0")
	}
	if p.EscalationMinIntervalSecs != nil && *p.EscalationMinIntervalSecs <= 0 {
		return fmt.Errorf("notify policy override escalation_min_interval_secs must be > 0")
	}

	return nil
}
