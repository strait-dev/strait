package domain

// ExpectedSchemaVersion is the schema version the binary expects. Each
// new migration bumps this constant so a binary/schema mismatch is
// caught at startup. Set to 0 to skip the check (useful in dev).
const ExpectedSchemaVersion = 219
