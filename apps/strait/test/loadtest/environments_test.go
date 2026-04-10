//go:build loadtest

package loadtest

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestEnvironments_Create(t *testing.T) {
	mustClean(t)
	projectID := "proj-env-create-" + newID()

	tgt := newTargeter("POST", "/v1/environments/", func() []byte {
		slug := "env-" + newID()
		return []byte(fmt.Sprintf(
			`{"project_id":"%s","name":"load-env-%s","slug":"%s","variables":{"KEY":"value","DB_HOST":"localhost"}}`,
			projectID, slug, slug,
		))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "create-environment", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "create-environment", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "create-environment", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestEnvironments_List(t *testing.T) {
	mustClean(t)
	projectID := "proj-env-list-" + newID()
	for range 15 {
		seedEnvironment(t, projectID)
	}

	tgt := newTargeter("GET", "/v1/environments/?project_id="+projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-environments", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-environments", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-environments", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestEnvironments_Get(t *testing.T) {
	mustClean(t)
	projectID := "proj-env-get-" + newID()
	envID := seedEnvironment(t, projectID)

	tgt := newTargeter("GET", "/v1/environments/"+envID+"/", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "get-environment", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "get-environment", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "get-environment", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestEnvironments_Update(t *testing.T) {
	mustClean(t)
	projectID := "proj-env-upd-" + newID()
	envID := seedEnvironment(t, projectID)

	var counter atomic.Int64
	tgt := newTargeter("PATCH", "/v1/environments/"+envID+"/", func() []byte {
		n := counter.Add(1)
		return []byte(fmt.Sprintf(`{"name":"updated-env-%d"}`, n))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "update-environment", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "update-environment", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "update-environment", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestEnvironments_Delete(t *testing.T) {
	mustClean(t)
	projectID := "proj-env-del-" + newID()

	envIDs := make([]string, 200)
	for i := range 200 {
		envIDs[i] = seedEnvironment(t, projectID)
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(envIDs))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/environments/" + envIDs[pos] + "/"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{testInternalSecret},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "delete-environment", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
}

func TestEnvironments_GetResolvedVariables(t *testing.T) {
	mustClean(t)
	projectID := "proj-env-vars-" + newID()
	envID := seedEnvironment(t, projectID)

	tgt := newTargeter("GET", "/v1/environments/"+envID+"/variables", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "get-resolved-vars", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "get-resolved-vars", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "get-resolved-vars", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestEnvironments_NestedInheritance(t *testing.T) {
	mustClean(t)
	projectID := "proj-env-nest-" + newID()

	parentSlug := "parent-" + newID()
	parentResp := httpDo(t, "POST", "/v1/environments/", fmt.Sprintf(
		`{"project_id":"%s","name":"parent","slug":"%s","variables":{"BASE_URL":"https://api.example.com","LOG_LEVEL":"info"}}`,
		projectID, parentSlug,
	), nil)
	parentID := parentResp["id"].(string)

	childSlug := "child-" + newID()
	childResp := httpDo(t, "POST", "/v1/environments/", fmt.Sprintf(
		`{"project_id":"%s","name":"child","slug":"%s","parent_id":"%s","variables":{"LOG_LEVEL":"debug","DB_HOST":"localhost"}}`,
		projectID, childSlug, parentID,
	), nil)
	childID := childResp["id"].(string)

	tgt := newTargeter("GET", "/v1/environments/"+childID+"/variables", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "nested-env-vars", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "nested-env-vars", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}
