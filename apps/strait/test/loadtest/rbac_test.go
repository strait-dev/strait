//go:build loadtest

package loadtest

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestRBAC_CreateRole(t *testing.T) {
	mustClean(t)

	tgt := newTargeter("POST", "/v1/roles", func() []byte {
		return []byte(fmt.Sprintf(
			`{"name":"load-role-%s","description":"load test role","permissions":["jobs:read","runs:read","stats:read"]}`,
			newID(),
		))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "create-role", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "create-role", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "create-role", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_ListRoles(t *testing.T) {
	mustClean(t)
	for range 10 {
		seedRole(t)
	}

	tgt := newTargeter("GET", "/v1/roles", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-roles", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-roles", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-roles", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_GetRole(t *testing.T) {
	mustClean(t)
	roleID := seedRole(t)

	tgt := newTargeter("GET", "/v1/roles/"+roleID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "get-role", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "get-role", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "get-role", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_UpdateRole(t *testing.T) {
	mustClean(t)
	roleID := seedRole(t)

	var counter atomic.Int64
	tgt := newTargeter("PATCH", "/v1/roles/"+roleID, func() []byte {
		n := counter.Add(1)
		return []byte(fmt.Sprintf(`{"description":"updated role description %d"}`, n))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "update-role", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "update-role", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "update-role", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_DeleteRole(t *testing.T) {
	mustClean(t)

	roleIDs := make([]string, 200)
	for i := range 200 {
		roleIDs[i] = seedRole(t)
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(roleIDs))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/roles/" + roleIDs[pos]
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "delete-role", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_AssignMember(t *testing.T) {
	mustClean(t)
	roleID := seedRole(t)

	tgt := newTargeter("POST", "/v1/members", func() []byte {
		return []byte(fmt.Sprintf(`{"user_id":"user-%s","role_id":"%s"}`, newID(), roleID))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "assign-member", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "assign-member", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_ListMembers(t *testing.T) {
	mustClean(t)
	for range 15 {
		seedMember(t)
	}

	tgt := newTargeter("GET", "/v1/members", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-members", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-members", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-members", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_RemoveMember(t *testing.T) {
	mustClean(t)

	userIDs := make([]string, 200)
	for i := range 200 {
		_, userIDs[i] = seedMember(t)
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(userIDs))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/members/" + userIDs[pos]
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "remove-member", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_CreateAPIKey(t *testing.T) {
	mustClean(t)
	projectID := "proj-apikey-" + newID()

	tgt := newTargeter("POST", "/v1/api-keys/", func() []byte {
		return []byte(fmt.Sprintf(
			`{"project_id":"%s","name":"load-key-%s","scopes":["jobs:read","runs:read"]}`,
			projectID, newID(),
		))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "create-api-key", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "create-api-key", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_ListAPIKeys(t *testing.T) {
	mustClean(t)
	projectID := "proj-apikey-list-" + newID()
	for range 10 {
		seedAPIKey(t, projectID, []string{"jobs:read"})
	}

	tgt := newTargeter("GET", "/v1/api-keys/?project_id="+projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-api-keys", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-api-keys", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_RotateAPIKey(t *testing.T) {
	mustClean(t)
	projectID := "proj-apikey-rot-" + newID()
	keyID, _ := seedAPIKey(t, projectID, []string{"jobs:read"})

	tgt := newTargeter("POST", "/v1/api-keys/"+keyID+"/rotate", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "rotate-api-key", tgt)
		assertSuccessRate(t, m, 0.99)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_RevokeAPIKey(t *testing.T) {
	mustClean(t)
	projectID := "proj-apikey-rev-" + newID()

	keyIDs := make([]string, 200)
	for i := range 200 {
		keyIDs[i], _ = seedAPIKey(t, projectID, []string{"jobs:read"})
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(keyIDs))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/api-keys/" + keyIDs[pos]
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "revoke-api-key", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_ListAuditEvents(t *testing.T) {
	mustClean(t)
	for range 10 {
		seedRole(t)
	}

	tgt := newTargeter("GET", "/v1/audit-events", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-audit-events", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-audit-events", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-audit-events", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_SeedSystemRoles(t *testing.T) {
	mustClean(t)

	tgt := newTargeter("POST", "/v1/seed-roles", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "seed-roles", tgt)
		assertSuccessRate(t, m, 0.99)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_CreateResourcePolicy(t *testing.T) {
	mustClean(t)
	roleID := seedRole(t)

	tgt := newTargeter("POST", "/v1/resource-policies", func() []byte {
		return []byte(fmt.Sprintf(
			`{"role_id":"%s","resource_type":"job","resource_id":"job-%s","permissions":["jobs:read"]}`,
			roleID, newID(),
		))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "create-resource-policy", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "create-resource-policy", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_ListResourcePolicies(t *testing.T) {
	mustClean(t)

	tgt := newTargeter("GET", "/v1/resource-policies", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-resource-policies", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-resource-policies", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_CreateTagPolicy(t *testing.T) {
	mustClean(t)
	roleID := seedRole(t)

	tgt := newTargeter("POST", "/v1/tag-policies", func() []byte {
		return []byte(fmt.Sprintf(
			`{"role_id":"%s","tag_key":"env","tag_value":"prod-%s","permissions":["jobs:read"]}`,
			roleID, newID(),
		))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "create-tag-policy", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "create-tag-policy", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_ListTagPolicies(t *testing.T) {
	mustClean(t)

	tgt := newTargeter("GET", "/v1/tag-policies", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-tag-policies", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-tag-policies", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_BulkAssignMembers(t *testing.T) {
	mustClean(t)
	roleID := seedRole(t)

	tgt := newTargeter("POST", "/v1/members/bulk", func() []byte {
		return []byte(fmt.Sprintf(
			`{"assignments":[{"user_id":"user-%s","role_id":"%s"},{"user_id":"user-%s","role_id":"%s"}]}`,
			newID(), roleID, newID(), roleID,
		))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "bulk-assign-members", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "bulk-assign-members", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestRBAC_APIKeyAuth(t *testing.T) {
	mustClean(t)
	projectID := "proj-apikey-auth-" + newID()
	seedJob(t, projectID)
	_, rawKey := seedAPIKey(t, projectID, []string{"jobs:read", "runs:read"})

	tgt := newAPIKeyTargeter("GET", "/v1/jobs/?project_id="+projectID, rawKey, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "api-key-auth", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "api-key-auth", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "api-key-auth", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}
