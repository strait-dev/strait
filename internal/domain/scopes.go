package domain

import "fmt"

// Scope constants define the available API key and RBAC permissions.
const (
	ScopeAll              = "*"
	ScopeJobsRead         = "jobs:read"
	ScopeJobsWrite        = "jobs:write"
	ScopeJobsTrigger      = "jobs:trigger"
	ScopeRunsRead         = "runs:read"
	ScopeRunsWrite        = "runs:write"
	ScopeWorkflowsRead    = "workflows:read"
	ScopeWorkflowsWrite   = "workflows:write"
	ScopeWorkflowsTrigger = "workflows:trigger"
	ScopeSecretsRead      = "secrets:read"
	ScopeSecretsWrite     = "secrets:write"
	ScopeAPIKeysManage    = "api-keys:manage"
	ScopeStatsRead        = "stats:read"
)

// ValidScopes is the set of all recognized scope strings.
var ValidScopes = map[string]bool{
	ScopeAll:              true,
	ScopeJobsRead:         true,
	ScopeJobsWrite:        true,
	ScopeJobsTrigger:      true,
	ScopeRunsRead:         true,
	ScopeRunsWrite:        true,
	ScopeWorkflowsRead:    true,
	ScopeWorkflowsWrite:   true,
	ScopeWorkflowsTrigger: true,
	ScopeSecretsRead:      true,
	ScopeSecretsWrite:     true,
	ScopeAPIKeysManage:    true,
	ScopeStatsRead:        true,
}

// ValidateScopes checks that all scopes in the slice are recognized.
func ValidateScopes(scopes []string) error {
	for _, s := range scopes {
		if !ValidScopes[s] {
			return fmt.Errorf("unknown scope: %q", s)
		}
	}
	return nil
}

// HasScope returns true if the given scopes slice contains the requested scope
// or the wildcard scope. An empty scopes slice is treated as wildcard access
// for backwards compatibility with pre-existing API keys.
func HasScope(scopes []string, required string) bool {
	if len(scopes) == 0 {
		return true // backwards compatible: empty = full access
	}
	for _, s := range scopes {
		if s == ScopeAll || s == required {
			return true
		}
	}
	return false
}
