//go:build loadtest

package loadtest

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestWebhooks_ListDeliveries(t *testing.T) {
	mustClean(t)
	projectID := "proj-whd-list-" + newID()
	seedJob(t, projectID)

	tgt := newTargeter("GET", "/v1/webhooks/deliveries/?project_id="+projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-webhook-deliveries", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-webhook-deliveries", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-webhook-deliveries", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWebhooks_ListDeliveriesLegacyRoute(t *testing.T) {
	mustClean(t)
	projectID := "proj-whd-leg-" + newID()
	seedJob(t, projectID)

	tgt := newTargeter("GET", "/v1/webhook-deliveries?project_id="+projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-webhook-deliveries-legacy", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-webhook-deliveries-legacy", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestWebhookSubscriptions_Create(t *testing.T) {
	mustClean(t)
	projectID := "proj-whs-create-" + newID()

	tgt := newTargeter("POST", "/v1/webhooks/subscriptions/", func() []byte {
		return []byte(fmt.Sprintf(
			`{"project_id":"%s","webhook_url":"https://example.com/wh-%s","event_types":["run.completed","run.failed"],"secret":"whsec-%s","active":true}`,
			projectID, newID(), newID(),
		))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "create-webhook-sub", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "create-webhook-sub", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "create-webhook-sub", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWebhookSubscriptions_List(t *testing.T) {
	mustClean(t)
	projectID := "proj-whs-list-" + newID()
	for range 15 {
		seedWebhookSubscription(t, projectID)
	}

	tgt := newTargeter("GET", "/v1/webhooks/subscriptions/?project_id="+projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-webhook-subs", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-webhook-subs", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-webhook-subs", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWebhookSubscriptions_Delete(t *testing.T) {
	mustClean(t)
	projectID := "proj-whs-del-" + newID()

	subIDs := make([]string, 200)
	for i := range 200 {
		subIDs[i] = seedWebhookSubscription(t, projectID)
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(subIDs))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/webhooks/subscriptions/" + subIDs[pos]
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "delete-webhook-sub", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
}
