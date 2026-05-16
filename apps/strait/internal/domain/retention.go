package domain

// MaxAuditRetentionDays bounds audit retention windows before code converts
// day counts into timestamps. It intentionally allows long enterprise
// retention while staying far below time.Duration overflow ranges.
const MaxAuditRetentionDays = 36500
