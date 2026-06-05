//go:build loadtest

package loadtest

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
		return fmt.Appendf(nil,
			`{"project_id":"%s","webhook_url":"https://example.com/wh-%s","event_types":["run.completed","run.failed"],"secret":"whsec-%s","active":true}`,
			projectID, newID(), newID(),
		)
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
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "delete-webhook-sub", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
}

func TestWebhooks_GetDelivery(t *testing.T) {
	mustClean(t)
	projectID := "proj-webhook-delivery-get-" + newID()
	jobID := seedJob(t, projectID)
	runID, _ := seedRun(t, jobID)

	deliveryIDs := make([]string, 200)
	ctx := context.Background()
	for i := range 200 {
		delivery := &domain.WebhookDelivery{
			RunID:       runID,
			JobID:       jobID,
			WebhookURL:  "https://example.com/webhooks/get-" + newID(),
			RetryPolicy: domain.WebhookRetryPolicyExponential,
			Status:      domain.WebhookStatusFailed,
			Attempts:    1,
			MaxAttempts: 3,
			LastError:   "failed delivery",
		}
		require.NoError(t,

			testStore.
				CreateWebhookDelivery(ctx, delivery))

		deliveryIDs[i] = delivery.ID
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(deliveryIDs))
		tgt.Method = "GET"
		tgt.URL = baseURL + "/v1/webhooks/deliveries/" + deliveryIDs[pos]
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "get-webhook-delivery", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "get-webhook-delivery", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "get-webhook-delivery", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWebhooks_RetryDelivery(t *testing.T) {
	mustClean(t)
	projectID := "proj-webhook-delivery-retry-" + newID()
	jobID := seedJob(t, projectID)
	runID, _ := seedRun(t, jobID)

	deliveryIDs := make([]string, 200)
	ctx := context.Background()
	now := time.Now().UTC()
	for i := range 200 {
		nextRetry := now.Add(time.Minute)
		delivery := &domain.WebhookDelivery{
			RunID:       runID,
			JobID:       jobID,
			WebhookURL:  "https://example.com/webhooks/retry-" + newID(),
			RetryPolicy: domain.WebhookRetryPolicyExponential,
			Status:      domain.WebhookStatusFailed,
			Attempts:    2,
			MaxAttempts: 5,
			LastError:   "retry candidate",
			NextRetryAt: &nextRetry,
		}
		require.NoError(t,

			testStore.
				CreateWebhookDelivery(ctx, delivery))

		deliveryIDs[i] = delivery.ID
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(deliveryIDs))
		tgt.Method = "POST"
		tgt.URL = baseURL + "/v1/webhook-deliveries/" + deliveryIDs[pos] + "/retry"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "retry-webhook-delivery", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "retry-webhook-delivery", tgt)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "retry-webhook-delivery", tgt)
		assertNoServerErrors(t, m)
	})
}
