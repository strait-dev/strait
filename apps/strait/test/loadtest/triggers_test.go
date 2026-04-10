//go:build loadtest

package loadtest

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestTriggers_BasicTrigger(t *testing.T) {
	mustClean(t)
	projectID := "proj-trig-basic-" + newID()
	jobID := seedJob(t, projectID)

	tgt := newTargeter("POST", "/v1/jobs/"+jobID+"/trigger", func() []byte {
		return []byte(fmt.Sprintf(`{"payload":{"id":"%s"}}`, newID()))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "basic-trigger", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "basic-trigger", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "basic-trigger", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestTriggers_WithUniqueIdempotencyKeys(t *testing.T) {
	mustClean(t)
	projectID := "proj-trig-idem-" + newID()
	jobID := seedJob(t, projectID)

	tgt := func(tgt *vegeta.Target) error {
		tgt.Method = "POST"
		tgt.URL = baseURL + "/v1/jobs/" + jobID + "/trigger"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{testInternalSecret},
			"Content-Type":      []string{"application/json"},
			"X-Idempotency-Key": []string{"idem-" + newID()},
		}
		tgt.Body = []byte(`{"payload":{}}`)
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "trigger-idem-unique", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "trigger-idem-unique", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "trigger-idem-unique", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestTriggers_SameIdempotencyKey(t *testing.T) {
	mustClean(t)
	projectID := "proj-trig-idem-same-" + newID()
	jobID := seedJob(t, projectID)
	fixedKey := "idem-fixed-" + newID()

	tgt := func(tgt *vegeta.Target) error {
		tgt.Method = "POST"
		tgt.URL = baseURL + "/v1/jobs/" + jobID + "/trigger"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{testInternalSecret},
			"Content-Type":      []string{"application/json"},
			"X-Idempotency-Key": []string{fixedKey},
		}
		tgt.Body = []byte(`{"payload":{"same":"key"}}`)
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "trigger-idem-same", tgt)
		assertSuccessRate(t, m, 0.99)
		assertStatusCodes(t, m, "201")
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "trigger-idem-same", tgt)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "trigger-idem-same", tgt)
		assertSuccessRate(t, m, 0.99)
	})
}

func TestTriggers_DelayedScheduling(t *testing.T) {
	mustClean(t)
	projectID := "proj-trig-delayed-" + newID()
	jobID := seedJob(t, projectID)

	tgt := newTargeter("POST", "/v1/jobs/"+jobID+"/trigger", func() []byte {
		future := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
		return []byte(fmt.Sprintf(`{"payload":{"delayed":true},"scheduled_at":"%s"}`, future))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "trigger-delayed", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "trigger-delayed", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "trigger-delayed", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestTriggers_WithPriority(t *testing.T) {
	mustClean(t)
	projectID := "proj-trig-prio-" + newID()
	jobID := seedJob(t, projectID)

	var counter atomic.Int64
	tgt := newTargeter("POST", "/v1/jobs/"+jobID+"/trigger", func() []byte {
		n := counter.Add(1)
		priority := n % 11
		return []byte(fmt.Sprintf(`{"payload":{"n":%d},"priority":%d}`, n, priority))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "trigger-priority", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "trigger-priority", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "trigger-priority", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestTriggers_BulkTrigger(t *testing.T) {
	mustClean(t)
	projectID := "proj-trig-bulk-" + newID()
	jobID := seedJob(t, projectID)

	tgt := newTargeter("POST", "/v1/jobs/"+jobID+"/trigger/bulk", func() []byte {
		items := ""
		for i := range 10 {
			if i > 0 {
				items += ","
			}
			items += fmt.Sprintf(`{"payload":{"i":%d},"idempotency_key":"blk-%s"}`, i, newID())
		}
		return []byte(fmt.Sprintf(`{"items":[%s]}`, items))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "bulk-trigger", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "bulk-trigger", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "bulk-trigger", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestTriggers_ConcurrentSameJob(t *testing.T) {
	mustClean(t)
	projectID := "proj-trig-conc-same-" + newID()
	jobID := seedJob(t, projectID)

	tgt := newTargeter("POST", "/v1/jobs/"+jobID+"/trigger", func() []byte {
		return []byte(fmt.Sprintf(`{"payload":{"ts":%d}}`, time.Now().UnixNano()))
	})

	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "concurrent-same-job", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestTriggers_ConcurrentDifferentJobs(t *testing.T) {
	mustClean(t)
	projectID := "proj-trig-conc-diff-" + newID()
	jobIDs := seedManyJobs(t, projectID, 10)

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		idx := counter.Add(1) % int64(len(jobIDs))
		tgt.Method = "POST"
		tgt.URL = baseURL + "/v1/jobs/" + jobIDs[idx] + "/trigger"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{testInternalSecret},
			"Content-Type":      []string{"application/json"},
		}
		tgt.Body = []byte(fmt.Sprintf(`{"payload":{"n":%d}}`, counter.Load()))
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "concurrent-diff-jobs", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "concurrent-diff-jobs", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "concurrent-diff-jobs", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestTriggers_AfterJobUpdate(t *testing.T) {
	mustClean(t)
	projectID := "proj-trig-upd-" + newID()
	jobID := seedJob(t, projectID)

	httpDo(t, "PATCH", "/v1/jobs/"+jobID+"/", `{"name":"updated-v2","max_attempts":5}`, nil)

	tgt := newTargeter("POST", "/v1/jobs/"+jobID+"/trigger", func() []byte {
		return []byte(fmt.Sprintf(`{"payload":{"ver":"v2","id":"%s"}}`, newID()))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "trigger-after-update", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "trigger-after-update", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "trigger-after-update", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestTriggers_LargePayload(t *testing.T) {
	mustClean(t)
	projectID := "proj-trig-large-" + newID()
	jobID := seedJob(t, projectID)

	largeValue := strings.Repeat("x", 10000)
	tgt := newTargeter("POST", "/v1/jobs/"+jobID+"/trigger", func() []byte {
		return []byte(fmt.Sprintf(`{"payload":{"id":"%s","data":"%s"}}`, newID(), largeValue))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "trigger-large-payload", tgt, withRate(50))
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "trigger-large-payload", tgt, withWorkers(20))
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "trigger-large-payload", tgt)
		assertSuccessRate(t, m, 0.85)
		assertNoServerErrors(t, m)
	})
}

func TestTriggers_RapidFire(t *testing.T) {
	mustClean(t)
	projectID := "proj-trig-rapid-" + newID()
	jobID := seedJob(t, projectID)

	tgt := newTargeter("POST", "/v1/jobs/"+jobID+"/trigger", func() []byte {
		return []byte(fmt.Sprintf(`{"payload":{"ts":%d}}`, time.Now().UnixNano()))
	})

	t.Run("rapid-baseline", func(t *testing.T) {
		m := runBaseline(t, "rapid-fire", tgt, withRate(500))
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("rapid-stress", func(t *testing.T) {
		m := runStress(t, "rapid-fire", tgt, withWorkers(100))
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}
