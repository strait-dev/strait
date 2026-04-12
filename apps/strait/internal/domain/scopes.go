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
	ScopeWebhooksRead     = "webhooks:read"
	ScopeWebhooksWrite    = "webhooks:write"
	ScopeAPIKeysManage    = "api-keys:manage"
	ScopeRBACManage       = "rbac:manage"
	ScopeStatsRead        = "stats:read"
	ScopeProjectsRead     = "projects:read"
	ScopeProjectsWrite    = "projects:write"
	ScopeProjectsManage   = "projects:manage"
	// DLQ admin scopes used by the admin DLQ HTTP endpoints.
	ScopeDLQRead   = "dlq:read"
	ScopeDLQReplay = "dlq:replay"
	ScopeDLQPurge  = "dlq:purge"
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
	ScopeWebhooksRead:     true,
	ScopeWebhooksWrite:    true,
	ScopeAPIKeysManage:    true,
	ScopeRBACManage:       true,
	ScopeStatsRead:        true,
	ScopeProjectsRead:     true,
	ScopeProjectsWrite:    true,
	ScopeProjectsManage:   true,
	ScopeDLQRead:          true,
	ScopeDLQReplay:        true,
	ScopeDLQPurge:         true,
}

// CLIDefaultScopes is the set of scopes granted to API keys created via CLI
// device-code authentication. It includes standard operational scopes but
// excludes administrative scopes (api-keys:manage, rbac:manage, projects:manage)
// to limit the blast radius of CLI-issued keys.
var CLIDefaultScopes = []string{
	ScopeJobsRead,
	ScopeJobsWrite,
	ScopeJobsTrigger,
	ScopeRunsRead,
	ScopeRunsWrite,
	ScopeWorkflowsRead,
	ScopeWorkflowsWrite,
	ScopeWorkflowsTrigger,
	ScopeSecretsRead,
	ScopeSecretsWrite,
	ScopeWebhooksRead,
	ScopeWebhooksWrite,
	ScopeStatsRead,
	ScopeProjectsRead,
	ScopeProjectsWrite,
	ScopeDLQRead,
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
