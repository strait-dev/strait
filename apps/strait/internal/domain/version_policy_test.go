package domain

import (
	"encoding/json"
	"slices"
	"testing"
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
			if got := tt.policy.IsValid(); got != tt.want {
				t.Errorf("VersionPolicy(%q).IsValid() = %v, want %v", tt.policy, got, tt.want)
			}
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
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var got wrapper
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got.Policy != policy {
				t.Fatalf("roundtrip: got %q, want %q", got.Policy, policy)
			}
		})
	}
}

func TestSystemRolePermissions_AdminHasWildcard(t *testing.T) {
	t.Parallel()

	perms := SystemRolePermissions["admin"]
	if len(perms) != 1 || perms[0] != "*" {
		t.Fatalf("admin permissions = %v, want [*]", perms)
	}
}

func TestSystemRolePermissions_ViewerCannotWrite(t *testing.T) {
	t.Parallel()

	viewerPerms := SystemRolePermissions["viewer"]
	for _, p := range viewerPerms {
		if p == ScopeJobsWrite || p == ScopeRunsWrite || p == ScopeWorkflowsWrite ||
			p == ScopeSecretsWrite || p == ScopeJobsTrigger || p == ScopeWorkflowsTrigger {
			t.Fatalf("viewer should not have write/trigger scope %q", p)
		}
	}
}

func TestSystemRolePermissions_OperatorHasRBACManage(t *testing.T) {
	t.Parallel()

	operatorPerms := SystemRolePermissions["operator"]
	if !slices.Contains(operatorPerms, ScopeRBACManage) {
		t.Fatal("operator should have rbac:manage permission")
	}
}

func TestSystemRolePermissions_TriggererCannotManageKeys(t *testing.T) {
	t.Parallel()

	triggererPerms := SystemRolePermissions["triggerer"]
	for _, p := range triggererPerms {
		if p == ScopeAPIKeysManage || p == ScopeRBACManage {
			t.Fatalf("triggerer should not have %q", p)
		}
	}
}

func TestSystemRolePermissions_AllRolesDefined(t *testing.T) {
	t.Parallel()

	expected := []string{"admin", "operator", "viewer", "triggerer"}
	for _, role := range expected {
		if _, ok := SystemRolePermissions[role]; !ok {
			t.Fatalf("system role %q missing from SystemRolePermissions", role)
		}
	}
	if len(SystemRolePermissions) != len(expected) {
		t.Fatalf("SystemRolePermissions has %d roles, want %d", len(SystemRolePermissions), len(expected))
	}
}

func TestSystemRolePermissions_NonAdminScopesAllValid(t *testing.T) {
	t.Parallel()

	// Every permission in non-admin roles should be a valid scope.
	for roleName, perms := range SystemRolePermissions {
		if roleName == "admin" {
			continue // admin has "*" which is special
		}
		for _, p := range perms {
			if !ValidScopes[p] {
				t.Errorf("role %q has invalid scope %q", roleName, p)
			}
		}
	}
}
