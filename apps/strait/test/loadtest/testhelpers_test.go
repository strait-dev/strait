//go:build loadtest

package loadtest

// testInternalSecret is the internal secret constant shared across load tests.
// Defined once to avoid magic string duplication.
const testInternalSecret = "test-secret-value"
