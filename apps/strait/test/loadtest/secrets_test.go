//go:build loadtest

package loadtest

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestSecrets_Create(t *testing.T) {
	mustClean(t)
	projectID := "proj-secret-create-" + newID()

	tgt := newTargeter("POST", "/v1/secrets/", func() []byte {
		secretKey := "secret-" + newID()
		return fmt.Appendf(nil,
			`{"project_id":"%s","secret_key":"%s","value":"val-%s"}`,
			projectID, secretKey, newID(),
		)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "create-secret", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "create-secret", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "create-secret", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestSecrets_List(t *testing.T) {
	mustClean(t)
	projectID := "proj-secret-list-" + newID()
	for range 20 {
		seedSecret(t, projectID)
	}

	tgt := newTargeter("GET", "/v1/secrets/?project_id="+projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-secrets", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-secrets", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-secrets", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestSecrets_Delete(t *testing.T) {
	mustClean(t)
	projectID := "proj-secret-delete-" + newID()
	secretIDs := make([]string, 200)
	for i := range 200 {
		secretIDs[i] = seedSecret(t, projectID)
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(secretIDs))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/secrets/" + secretIDs[pos]
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "delete-secret", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
}
