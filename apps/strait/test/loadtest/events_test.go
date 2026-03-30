//go:build loadtest

package loadtest

import (
	"fmt"
	"net/http"
	"testing"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestEvents_ListEventTriggers(t *testing.T) {
	mustClean(t)
	projectID := "proj-evt-list-" + newID()
	for range 10 {
		seedEventTrigger(t, projectID)
	}

	tgt := newProjectTargeter("GET", "/v1/events/", projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-event-triggers", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-event-triggers", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-event-triggers", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestEvents_GetEventTrigger(t *testing.T) {
	mustClean(t)
	projectID := "proj-evt-get-" + newID()
	_, eventKey := seedEventTrigger(t, projectID)

	tgt := newProjectTargeter("GET", "/v1/events/"+eventKey, projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "get-event-trigger", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "get-event-trigger", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "get-event-trigger", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestEvents_GetEventTriggerStats(t *testing.T) {
	mustClean(t)
	projectID := "proj-evt-stats-" + newID()
	for range 10 {
		seedEventTrigger(t, projectID)
	}

	tgt := newProjectTargeter("GET", "/v1/events/stats", projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "event-trigger-stats", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "event-trigger-stats", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "event-trigger-stats", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestEvents_SendEvent(t *testing.T) {
	mustClean(t)
	projectID := "proj-evt-send-" + newID()
	_, eventKey := seedEventTrigger(t, projectID)

	tgt := newProjectTargeter("POST", "/v1/events/"+eventKey+"/send", projectID, func() []byte {
		return []byte(fmt.Sprintf(`{"payload":{"data":"%s"}}`, newID()))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "send-event", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "send-event", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestEvents_SendEventByPrefix(t *testing.T) {
	mustClean(t)
	projectID := "proj-evt-pfx-" + newID()
	for range 5 {
		seedEventTrigger(t, projectID)
	}

	tgt := newProjectTargeter("POST", "/v1/events/prefix/load/send", projectID, func() []byte {
		return []byte(fmt.Sprintf(`{"payload":{"data":"%s"}}`, newID()))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "send-event-prefix", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "send-event-prefix", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestEvents_CancelEventTrigger(t *testing.T) {
	mustClean(t)
	projectID := "proj-evt-cancel-" + newID()

	eventKeys := make([]string, 200)
	for i := range 200 {
		_, eventKeys[i] = seedEventTrigger(t, projectID)
	}

	var counter int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter
		counter++
		pos := i % int64(len(eventKeys))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/events/" + eventKeys[pos]
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"X-Project-Id":      []string{projectID},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "cancel-event-trigger", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
}

func TestEvents_PurgeEventTriggers(t *testing.T) {
	mustClean(t)
	projectID := "proj-evt-purge-" + newID()
	for range 10 {
		seedEventTrigger(t, projectID)
	}

	tgt := newProjectTargeter("POST", "/v1/events/purge", projectID, func() []byte {
		return []byte(fmt.Sprintf(`{"project_id":"%s","older_than_hours":0}`, projectID))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "purge-events", tgt)
		assertSuccessRate(t, m, 0.99)
		assertNoServerErrors(t, m)
	})
}
