//go:build integration

package e2e_test

// testInternalSecret is the internal secret constant shared across e2e
// integration tests. Defined once to avoid magic string duplication.
const testInternalSecret = "test-secret-value"
