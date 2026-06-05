package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		scopes  []string
		wantErr bool
	}{
		{"empty is valid", []string{}, false},
		{"nil is valid", nil, false},
		{"wildcard", []string{"*"}, false},
		{"single valid", []string{"jobs:read"}, false},
		{"multiple valid", []string{"jobs:read", "runs:write", "workflows:trigger"}, false},
		{"unknown scope", []string{"foo:bar"}, true},
		{"mix valid and invalid", []string{"jobs:read", "invalid"}, true},
		{"duplicate scopes valid", []string{"jobs:read", "jobs:read"}, false},
		{"empty string scope invalid", []string{""}, true},
		{"case sensitive - uppercase fails", []string{"Jobs:Read"}, true},
		{"case sensitive - UPPER fails", []string{"JOBS:READ"}, true},
		{"whitespace scope invalid", []string{" jobs:read "}, true},
		{"whitespace prefix invalid", []string{" jobs:read"}, true},
		{"all valid scopes individually", []string{"jobs:read"}, false},
		{"partial scope name", []string{"jobs"}, true},
		{"colon only", []string{":"}, true},
		{"scope with extra colon", []string{"jobs:read:extra"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateScopes(tt.scopes)
			assert.Equal(t,
				tt.wantErr,
				(err !=
					nil))
		})
	}
}

func TestValidateScopes_AllConstants(t *testing.T) {
	t.Parallel()

	// Every scope constant should pass validation individually.
	allScopes := []string{
		ScopeAll, ScopeJobsRead, ScopeJobsWrite, ScopeJobsTrigger,
		ScopeRunsRead, ScopeRunsWrite,
		ScopeWorkflowsRead, ScopeWorkflowsWrite, ScopeWorkflowsTrigger,
		ScopeSecretsRead, ScopeSecretsWrite,
		ScopeWebhooksRead, ScopeWebhooksWrite,
		ScopeAPIKeysManage, ScopeRBACManage, ScopeStatsRead,
		ScopeProjectsRead, ScopeProjectsWrite, ScopeProjectsManage,
		ScopeWorkersConnect,
		ScopeDLQRead, ScopeDLQReplay, ScopeDLQPurge,
		ScopeOutboxRead, ScopeOutboxRetry, ScopeOutboxPurge,
	}
	for _, scope := range allScopes {
		require.NoError(t,
			ValidateScopes([]string{scope}))
	}

	// Every constant should also be in ValidScopes map.
	for _, scope := range allScopes {
		assert.True(t, ValidScopes[scope])
	}
	assert.Len(t, ValidScopes,

		len(allScopes))

	// ValidScopes map should have same count as allScopes.
}

func TestValidateScopes_ErrorMessage(t *testing.T) {
	t.Parallel()

	err := ValidateScopes([]string{"banana"})
	require.Error(t,
		err)
	assert.Contains(t, err.Error(), "banana")
}

func TestHasScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		scopes   []string
		required string
		want     bool
	}{
		{"empty scopes allows all", []string{}, "jobs:read", true},
		{"wildcard allows all", []string{"*"}, "jobs:write", true},
		{"exact match", []string{"jobs:read"}, "jobs:read", true},
		{"no match", []string{"jobs:read"}, "jobs:write", false},
		{"multiple with match", []string{"jobs:read", "runs:read"}, "runs:read", true},
		{"multiple without match", []string{"jobs:read", "runs:read"}, "workflows:write", false},
		{"wildcard among others", []string{"jobs:read", "*"}, "anything", true},
		{"required is wildcard with scopes", []string{"jobs:read"}, "*", false},
		{"required is empty string", []string{"jobs:read"}, "", false},
		{"wildcard scope with empty required", []string{"*"}, "", true},
		{"large scope list last match", makeLargeScopes(100, "target:scope"), "target:scope", true},
		{"large scope list no match", makeLargeScopes(100, ""), "target:scope", false},
		{"nil scopes allows all", nil, "jobs:read", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := HasScope(tt.scopes, tt.required)
			assert.Equal(t,
				tt.want, got,
			)
		})
	}
}

func TestHasScopeStrict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		scopes   []string
		required string
		want     bool
	}{
		{"nil scopes deny", nil, ScopeJobsRead, false},
		{"empty scopes deny", []string{}, ScopeJobsRead, false},
		{"wildcard allows all", []string{ScopeAll}, ScopeJobsWrite, true},
		{"exact match", []string{ScopeRunsRead}, ScopeRunsRead, true},
		{"no match", []string{ScopeRunsRead}, ScopeRunsWrite, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, HasScopeStrict(tt.scopes, tt.required))
		})
	}
}

func makeLargeScopes(n int, lastScope string) []string {
	scopes := make([]string, 0, n)
	for i := range n - 1 {
		scopes = append(scopes, "scope:"+strings.Repeat("x", i%10))
	}
	if lastScope != "" {
		scopes = append(scopes, lastScope)
	} else {
		scopes = append(scopes, "scope:zzz")
	}
	return scopes
}
