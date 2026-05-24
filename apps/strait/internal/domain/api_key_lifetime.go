package domain

import (
	"fmt"
	"time"
)

// MaxAPIKeyDurationDays caps API key lifetime settings at 100 years.
// time.Duration(days)*24*time.Hour overflows once days exceeds ~106,750.
const MaxAPIKeyDurationDays = 36500

func ApplyAPIKeyLifetimePolicy(now time.Time, requested *time.Time, maxKeyLifetimeDays int) (*time.Time, error) {
	expiresAt := requested
	if maxKeyLifetimeDays > 0 {
		effectiveMaxDays := min(maxKeyLifetimeDays, MaxAPIKeyDurationDays)
		maxExpiry := now.Add(time.Duration(effectiveMaxDays) * 24 * time.Hour)
		if expiresAt == nil {
			expiresAt = &maxExpiry
		} else if expiresAt.After(maxExpiry) {
			return nil, fmt.Errorf("expires_in_days exceeds project maximum of %d days", effectiveMaxDays)
		}
	}
	if expiresAt == nil {
		return nil, fmt.Errorf("expires_in_days is required when project max_key_lifetime_days is not configured")
	}
	return expiresAt, nil
}
