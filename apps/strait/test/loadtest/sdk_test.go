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

// SDK seeding helpers

// seedExecutingRun creates a run and transitions it to executing status,
// returning (runID, runToken). SDK endpoints typically require executing runs.
func seedExecutingRun(t *testing.T, jobID string) (runID, runToken string) {
	t.Helper()
	runID, runToken = seedRun(t, jobID)
	ctx := context.Background()

	err := testStore.UpdateRunStatus(ctx, runID, domain.StatusQueued, domain.StatusDequeued, map[string]any{
		"started_at": time.Now().UTC(),
	})
	require.NoError(t,

		err)

	err = testStore.UpdateRunStatus(ctx, runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{})
	require.NoError(t,

		err)

	return runID, runToken
}

// seedManyExecutingRuns creates n executing runs and returns parallel slices of
// (runIDs, runTokens).
func seedManyExecutingRuns(t *testing.T, jobID string, n int) ([]string, []string) {
	t.Helper()
	ids := make([]string, n)
	tokens := make([]string, n)
	for i := range n {
		ids[i], tokens[i] = seedExecutingRun(t, jobID)
		if i > 0 && i%100 == 0 {
			t.Logf("seeded %d/%d executing runs", i, n)
		}
	}
	return ids, tokens
}

// SDK Log

func TestSDK_Log(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-log-" + newID()
	jobID := seedJob(t, projectID)
	runID, runToken := seedExecutingRun(t, jobID)

	tgt := newSDKTargeter("POST", "/sdk/v1/runs/"+runID+"/log", runToken, func() []byte {
		return fmt.Appendf(nil, `{"message":"load test log %s","level":"info","data":{"iter":true}}`, newID())
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-log", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "sdk-log", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "sdk-log", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

// SDK Progress

func TestSDK_Progress(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-prog-" + newID()
	jobID := seedJob(t, projectID)
	runID, runToken := seedExecutingRun(t, jobID)

	var counter atomic.Int64
	tgt := newSDKTargeter("POST", "/sdk/v1/runs/"+runID+"/progress", runToken, func() []byte {
		n := counter.Add(1)
		pct := float64(n%100) + 0.5
		return fmt.Appendf(nil, `{"percent":%.1f,"message":"step %d","step":"phase-%d","eta_seconds":%d}`,
			pct, n, n%5, 60-n%60)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-progress", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "sdk-progress", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "sdk-progress", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

// SDK Annotate

func TestSDK_Annotate(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-ann-" + newID()
	jobID := seedJob(t, projectID)
	runID, runToken := seedExecutingRun(t, jobID)

	var counter atomic.Int64
	tgt := newSDKTargeter("POST", "/sdk/v1/runs/"+runID+"/annotate", runToken, func() []byte {
		n := counter.Add(1)
		return fmt.Appendf(nil, `{"annotations":{"env":"prod","region":"us-east-%d","version":"v%d"}}`, n%4, n)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-annotate", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "sdk-annotate", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "sdk-annotate", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

// SDK Heartbeat

func TestSDK_Heartbeat(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-hb-" + newID()
	jobID := seedJob(t, projectID)
	runID, runToken := seedExecutingRun(t, jobID)

	tgt := newSDKTargeter("POST", "/sdk/v1/runs/"+runID+"/heartbeat", runToken, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-heartbeat", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "sdk-heartbeat", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "sdk-heartbeat", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

// SDK Checkpoint

func TestSDK_Checkpoint(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-ckpt-" + newID()
	jobID := seedJob(t, projectID)
	runID, runToken := seedExecutingRun(t, jobID)

	var counter atomic.Int64
	tgt := newSDKTargeter("POST", "/sdk/v1/runs/"+runID+"/checkpoint", runToken, func() []byte {
		n := counter.Add(1)
		return fmt.Appendf(nil, `{"state":{"cursor":%d,"batch_size":100,"offset":%d}}`, n, n*100)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-checkpoint", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "sdk-checkpoint", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "sdk-checkpoint", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

// SDK Output

func TestSDK_Output(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-out-" + newID()
	jobID := seedJob(t, projectID)
	runID, runToken := seedExecutingRun(t, jobID)

	var counter atomic.Int64
	tgt := newSDKTargeter("POST", "/sdk/v1/runs/"+runID+"/output", runToken, func() []byte {
		n := counter.Add(1)
		return fmt.Appendf(nil,
			`{"output_key":"result-%d","value":{"score":%d,"label":"output-%d"}}`,
			n, n%100, n,
		)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-output", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "sdk-output", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "sdk-output", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

// SDK Complete

func TestSDK_Complete(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-comp-" + newID()
	jobID := seedJob(t, projectID)

	// Each complete request consumes a run (terminal transition), so we need
	// a pool of executing runs and rotate through them. After all are consumed,
	// subsequent requests will 404/409 — that's expected under stress.
	const poolSize = 500
	runIDs, runTokens := seedManyExecutingRuns(t, jobID, poolSize)

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(runIDs))
		tgt.Method = "POST"
		tgt.URL = baseURL + "/sdk/v1/runs/" + runIDs[pos] + "/complete"
		tgt.Header = http.Header{
			"Authorization": []string{"Bearer " + runTokens[pos]},
			"Content-Type":  []string{"application/json"},
		}
		tgt.Body = []byte(`{"result":{"ok":true}}`)
		return nil
	}

	// Only baseline — complete is a destructive operation. High success rate
	// only for the first poolSize requests; after that expect conflicts.
	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-complete", tgt, withRate(50), withDuration(8*time.Second))
		assertNoServerErrors(t, m)
	})
}

// SDK Fail

func TestSDK_Fail(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-fail-" + newID()
	jobID := seedJob(t, projectID)

	const poolSize = 500
	runIDs, runTokens := seedManyExecutingRuns(t, jobID, poolSize)

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(runIDs))
		tgt.Method = "POST"
		tgt.URL = baseURL + "/sdk/v1/runs/" + runIDs[pos] + "/fail"
		tgt.Header = http.Header{
			"Authorization": []string{"Bearer " + runTokens[pos]},
			"Content-Type":  []string{"application/json"},
		}
		tgt.Body = fmt.Appendf(nil, `{"error":"load test failure %d"}`, i)
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-fail", tgt, withRate(50), withDuration(8*time.Second))
		assertNoServerErrors(t, m)
	})
}

// SDK Spawn

func TestSDK_Spawn(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-spawn-" + newID()
	// Parent job for spawning
	parentJobID := seedJob(t, projectID)
	// Child job that gets spawned — must exist before spawn call
	childSlug := "child-" + newID()
	httpDo(t, "POST", "/v1/jobs/", fmt.Sprintf(
		`{"project_id":"%s","name":"child-%s","slug":"%s","endpoint_url":"https://example.com/child","max_attempts":1,"timeout_secs":30}`,
		projectID, childSlug, childSlug,
	), nil)

	runID, runToken := seedExecutingRun(t, parentJobID)

	tgt := newSDKTargeter("POST", "/sdk/v1/runs/"+runID+"/spawn", runToken, func() []byte {
		return fmt.Appendf(nil,
			`{"job_slug":"%s","project_id":"%s","payload":{"spawned":"%s"}}`,
			childSlug, projectID, newID(),
		)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-spawn", tgt, withRate(30), withDuration(8*time.Second))
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "sdk-spawn", tgt, withRate(200), withWorkers(20))
		assertNoServerErrors(t, m)
	})
}

// SDK Continue

func TestSDK_Continue(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-cont-" + newID()
	jobID := seedJob(t, projectID)

	// Continue is a terminal-like operation: it completes the current run and
	// queues a continuation. Each run can only be continued once.
	const poolSize = 300
	runIDs, runTokens := seedManyExecutingRuns(t, jobID, poolSize)

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(runIDs))
		tgt.Method = "POST"
		tgt.URL = baseURL + "/sdk/v1/runs/" + runIDs[pos] + "/continue"
		tgt.Header = http.Header{
			"Authorization": []string{"Bearer " + runTokens[pos]},
			"Content-Type":  []string{"application/json"},
		}
		tgt.Body = fmt.Appendf(nil, `{"payload":{"step":%d}}`, i)
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-continue", tgt, withRate(30), withDuration(8*time.Second))
		assertNoServerErrors(t, m)
	})
}

// SDK WaitForEvent

func TestSDK_WaitForEvent(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-wfe-" + newID()
	jobID := seedJob(t, projectID)

	// Each wait-for-event transitions run to waiting (one-time operation per run),
	// and requires unique event keys. Use a pool of executing runs.
	const poolSize = 200
	runIDs, runTokens := seedManyExecutingRuns(t, jobID, poolSize)

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(runIDs))
		tgt.Method = "POST"
		tgt.URL = baseURL + "/sdk/v1/runs/" + runIDs[pos] + "/wait-for-event"
		tgt.Header = http.Header{
			"Authorization": []string{"Bearer " + runTokens[pos]},
			"Content-Type":  []string{"application/json"},
		}
		// Each event key must be unique to avoid conflicts.
		tgt.Body = fmt.Appendf(nil, `{"event_key":"load:%s","timeout_secs":3600}`, newID())
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-wait-for-event", tgt, withRate(20), withDuration(8*time.Second))
		assertNoServerErrors(t, m)
	})
}

// SDK Concurrent Multi-Endpoint (mixed workload)

func TestSDK_ConcurrentMixedOperations(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-mix-" + newID()
	jobID := seedJob(t, projectID)
	runID, runToken := seedExecutingRun(t, jobID)

	// Rotate through non-destructive SDK operations on the same run.
	endpoints := []struct {
		path string
		body string
	}{
		{"/sdk/v1/runs/" + runID + "/log", `{"message":"mixed load test"}`},
		{"/sdk/v1/runs/" + runID + "/heartbeat", ""},
		{"/sdk/v1/runs/" + runID + "/progress", `{"percent":50,"message":"halfway"}`},
		{"/sdk/v1/runs/" + runID + "/checkpoint", `{"state":{"cursor":1}}`},
		{"/sdk/v1/runs/" + runID + "/output", `{"output_key":"mixed","value":{"ok":true}}`},
		{"/sdk/v1/runs/" + runID + "/annotate", `{"annotations":{"env":"test"}}`},
	}

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		ep := endpoints[i%int64(len(endpoints))]
		tgt.Method = "POST"
		tgt.URL = baseURL + ep.path
		tgt.Header = http.Header{
			"Authorization": []string{"Bearer " + runToken},
			"Content-Type":  []string{"application/json"},
		}
		if ep.body != "" {
			tgt.Body = []byte(ep.body)
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-mixed", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "sdk-mixed", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "sdk-mixed", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

// SDK Rapid Heartbeat (high-frequency single-endpoint)

func TestSDK_RapidHeartbeat(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-rhb-" + newID()
	jobID := seedJob(t, projectID)
	runID, runToken := seedExecutingRun(t, jobID)

	tgt := newSDKTargeter("POST", "/sdk/v1/runs/"+runID+"/heartbeat", runToken, nil)

	// Heartbeat is expected to handle very high frequency from long-running jobs.
	t.Run("baseline-high-freq", func(t *testing.T) {
		m := runBaseline(t, "sdk-rapid-heartbeat", tgt, withRate(500))
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "sdk-rapid-heartbeat", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

// SDK Multi-Run Log Fan-Out (many runs logging concurrently)

func TestSDK_MultiRunLogFanOut(t *testing.T) {
	mustClean(t)
	projectID := "proj-sdk-fanout-" + newID()
	jobID := seedJob(t, projectID)

	const numRuns = 50
	runIDs, runTokens := seedManyExecutingRuns(t, jobID, numRuns)

	var counter atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := counter.Add(1) - 1
		pos := i % int64(len(runIDs))
		tgt.Method = "POST"
		tgt.URL = baseURL + "/sdk/v1/runs/" + runIDs[pos] + "/log"
		tgt.Header = http.Header{
			"Authorization": []string{"Bearer " + runTokens[pos]},
			"Content-Type":  []string{"application/json"},
		}
		tgt.Body = fmt.Appendf(nil, `{"message":"fan-out log %d from run %d"}`, i, pos)
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "sdk-log-fanout", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "sdk-log-fanout", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "sdk-log-fanout", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}
