package domain

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionPolicy_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		policy VersionPolicy
		want   bool
	}{
		{VersionPolicyPin, true},
		{VersionPolicyLatest, true},
		{VersionPolicyMinor, true},
		{VersionPolicy(""), false},
		{VersionPolicy("invalid"), false},
		{VersionPolicy("PIN"), false},
		{VersionPolicy("Latest"), false},
		{VersionPolicy(" pin"), false},
	}

	for _, tt := range tests {
		name := string(tt.policy)
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.policy.IsValid())
		})
	}
}

func TestVersionPolicy_JSONRoundtrip(t *testing.T) {
	t.Parallel()

	type wrapper struct {
		Policy VersionPolicy `json:"policy"`
	}

	for _, policy := range []VersionPolicy{VersionPolicyPin, VersionPolicyLatest, VersionPolicyMinor} {
		t.Run(string(policy), func(t *testing.T) {
			t.Parallel()
			w := wrapper{Policy: policy}
			data, err := json.Marshal(w)
			require.NoError(t, err)

			var got wrapper
			require.NoError(t, json.
				Unmarshal(data, &got),
			)
			require.Equal(t, policy,
				got.Policy,
			)
		})
	}
}

func TestSystemRolePermissions_AdminHasWildcard(t *testing.T) {
	t.Parallel()

	perms := SystemRolePermissions["admin"]
	require.False(t, len(perms) != 1 ||
		perms[0] !=
			"*")
}

func TestSystemRolePermissions_ViewerCannotWrite(t *testing.T) {
	t.Parallel()

	viewerPerms := SystemRolePermissions["viewer"]
	for _, p := range viewerPerms {
		require.False(t, p ==
			ScopeJobsWrite ||
			p ==

				ScopeRunsWrite ||
			p == ScopeWorkflowsWrite || p ==
			ScopeSecretsWrite ||
			p == ScopeJobsTrigger ||
			p == ScopeWorkflowsTrigger,
		)
	}
}

func TestSystemRolePermissions_OperatorHasRBACManage(t *testing.T) {
	t.Parallel()

	operatorPerms := SystemRolePermissions["operator"]
	require.True(t, slices.
		Contains(
			operatorPerms,

			ScopeRBACManage))
}

func TestSystemRolePermissions_OperatorHasOutboxMutationScopes(t *testing.T) {
	t.Parallel()

	operatorPerms := SystemRolePermissions["operator"]
	for _, scope := range []string{ScopeOutboxRead, ScopeOutboxRetry, ScopeOutboxPurge} {
		require.True(t, slices.
			Contains(
				operatorPerms,

				scope))
	}
}

func TestSystemRolePermissions_TriggererCannotManageKeys(t *testing.T) {
	t.Parallel()

	triggererPerms := SystemRolePermissions["triggerer"]
	for _, p := range triggererPerms {
		require.False(t, p ==
			ScopeAPIKeysManage ||
			p ==
				ScopeRBACManage)
	}
}

func TestSystemRolePermissions_AllRolesDefined(t *testing.T) {
	t.Parallel()

	expected := []string{"admin", "operator", "viewer", "triggerer"}
	for _, role := range expected {
		require.Contains(t, SystemRolePermissions, role)
	}
	require.Len(t, SystemRolePermissions,

		len(expected))
}

func TestSystemRolePermissions_NonAdminScopesAllValid(t *testing.T) {
	t.Parallel()

	// Every permission in non-admin roles should be a valid scope.
	for roleName, perms := range SystemRolePermissions {
		if roleName == "admin" {
			continue // admin has "*" which is special
		}
		for _, p := range perms {
			assert.True(t, ValidScopes[p])
		}
	}
}
