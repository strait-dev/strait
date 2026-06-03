//go:build loadtest

package loadtest

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestRuns_ListRuns(t *testing.T) {
	mustClean(t)
	projectID := "proj-lr-" + newID()
	jobID := seedJob(t, projectID)
	seedManyRuns(t, jobID, 50)

	tgt := newTargeter("GET", "/v1/runs/?project_id="+projectID+"&limit=50", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-runs", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-runs", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-runs", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_ListRunsByStatus(t *testing.T) {
	mustClean(t)
	projectID := "proj-lrs-" + newID()
	jobID := seedJob(t, projectID)
	seedManyRuns(t, jobID, 30)

	tgt := newTargeter("GET", "/v1/runs/?project_id="+projectID+"&status=queued", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-runs-status", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-runs-status", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-runs-status", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_ListRunsPaginated(t *testing.T) {
	mustClean(t)
	projectID := "proj-lrp-" + newID()
	jobID := seedJob(t, projectID)
	seedManyRuns(t, jobID, 50)

	tgt := newTargeter("GET", "/v1/runs/?project_id="+projectID+"&limit=10", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-runs-paginated", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-runs-paginated", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-runs-paginated", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_GetRun(t *testing.T) {
	mustClean(t)
	projectID := "proj-gr-" + newID()
	jobID := seedJob(t, projectID)
	runID, _ := seedRun(t, jobID)

	tgt := newTargeter("GET", "/v1/runs/"+runID+"/", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "get-run", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "get-run", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "get-run", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_CancelRun(t *testing.T) {
	mustClean(t)
	projectID := "proj-cr-" + newID()
	jobID := seedJob(t, projectID)
	runIDs := seedManyRuns(t, jobID, 200)

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		i %= int64(len(runIDs))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/runs/" + runIDs[i] + "/"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "cancel-run", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "cancel-run", tgt)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "cancel-run", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_ReplayRun(t *testing.T) {
	mustClean(t)
	projectID := "proj-rr-" + newID()
	jobID := seedJob(t, projectID)

	failedIDs := make([]string, 50)
	for i := range 50 {
		failedIDs[i] = seedRunTerminal(t, jobID, domain.StatusFailed)
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		i %= int64(len(failedIDs))
		tgt.Method = "POST"
		tgt.URL = baseURL + "/v1/runs/" + failedIDs[i] + "/replay"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "replay-run", tgt, withRate(30))
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "replay-run", tgt, withWorkers(20))
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "replay-run", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_ListDLQ(t *testing.T) {
	mustClean(t)
	projectID := "proj-dlq-" + newID()
	jobID := seedJob(t, projectID)
	ctx := context.Background()

	for range 20 {
		id, _ := seedRun(t, jobID)
		_ = testStore.UpdateRunStatus(ctx, id, domain.StatusQueued, domain.StatusDequeued, map[string]any{
			"started_at": time.Now().UTC(),
		})
		_ = testStore.UpdateRunStatus(ctx, id, domain.StatusDequeued, domain.StatusExecuting, map[string]any{})
		_ = testStore.UpdateRunStatus(ctx, id, domain.StatusExecuting, domain.StatusDeadLetter, map[string]any{
			"finished_at": time.Now().UTC(),
			"error":       "max attempts exceeded",
		})
	}

	tgt := newTargeter("GET", "/v1/runs/dlq?project_id="+projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-dlq", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-dlq", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-dlq", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_BulkCancel(t *testing.T) {
	mustClean(t)
	projectID := "proj-bc-" + newID()
	jobID := seedJob(t, projectID)
	runIDs := seedManyRuns(t, jobID, 100)

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := int(idx.Add(1)-1) * 5
		if i+5 > len(runIDs) {
			i %= len(runIDs) - 4
		}
		var ids strings.Builder
		for j := range 5 {
			if j > 0 {
				ids.WriteString(",")
			}
			_, _ = fmt.Fprintf(&ids, `"%s"`, runIDs[i+j])
		}
		tgt.Method = "POST"
		tgt.URL = baseURL + "/v1/runs/bulk-cancel"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
		}
		tgt.Body = fmt.Appendf(nil, `{"run_ids":[%s]}`, ids.String())
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "bulk-cancel", tgt, withRate(20))
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "bulk-cancel", tgt, withWorkers(10))
		assertNoServerErrors(t, m)
	})
}

func TestRuns_ListRunEvents(t *testing.T) {
	mustClean(t)
	projectID := "proj-re-" + newID()
	jobID := seedJob(t, projectID)
	runID, runToken := seedExecutingRun(t, jobID)

	for i := range 10 {
		httpDo(t, "POST", "/sdk/v1/runs/"+runID+"/log", fmt.Sprintf(
			`{"type":"log","level":"info","message":"Step %d done","data":{"step":%d}}`, i, i,
		), http.Header{"Authorization": []string{"Bearer " + runToken}})
	}

	tgt := newTargeter("GET", "/v1/runs/"+runID+"/events", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-run-events", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-run-events", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-run-events", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_ListChildRuns(t *testing.T) {
	mustClean(t)
	projectID := "proj-child-" + newID()
	jobID := seedJob(t, projectID)
	runID, _ := seedRun(t, jobID)

	tgt := newTargeter("GET", "/v1/runs/"+runID+"/children", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-children", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-children", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-children", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_ListCheckpoints(t *testing.T) {
	mustClean(t)
	projectID := "proj-ckpt-" + newID()
	jobID := seedJob(t, projectID)
	runID, runToken := seedExecutingRun(t, jobID)

	for i := range 5 {
		httpDo(t, "POST", "/sdk/v1/runs/"+runID+"/checkpoint", fmt.Sprintf(
			`{"state":{"step":%d,"progress":%.1f}}`, i, float64(i)*20.0,
		), http.Header{"Authorization": []string{"Bearer " + runToken}})
	}

	tgt := newTargeter("GET", "/v1/runs/"+runID+"/checkpoints", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-checkpoints", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-checkpoints", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-checkpoints", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_ListOutputs(t *testing.T) {
	mustClean(t)
	projectID := "proj-out-" + newID()
	jobID := seedJob(t, projectID)
	runID, runToken := seedExecutingRun(t, jobID)

	for i := range 3 {
		httpDo(t, "POST", "/sdk/v1/runs/"+runID+"/output", fmt.Sprintf(
			`{"output_key":"result-%d","value":{"data":"output-%d"}}`, i, i,
		), http.Header{"Authorization": []string{"Bearer " + runToken}})
	}

	tgt := newTargeter("GET", "/v1/runs/"+runID+"/outputs", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-outputs", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-outputs", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-outputs", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_ListByMetadata(t *testing.T) {
	mustClean(t)
	projectID := "proj-meta-" + newID()
	jobID := seedJob(t, projectID)

	for range 10 {
		id, token := seedExecutingRun(t, jobID)
		httpDo(t, "POST", "/sdk/v1/runs/"+id+"/annotate",
			`{"annotations":{"env":"prod","region":"us-east"}}`,
			http.Header{"Authorization": []string{"Bearer " + token}})
	}
	for range 5 {
		id, token := seedExecutingRun(t, jobID)
		httpDo(t, "POST", "/sdk/v1/runs/"+id+"/annotate",
			`{"annotations":{"env":"staging"}}`,
			http.Header{"Authorization": []string{"Bearer " + token}})
	}

	tgt := newTargeter("GET", "/v1/runs/?project_id="+projectID+"&metadata_key=env&metadata_value=prod", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-by-metadata", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-by-metadata", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-by-metadata", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_DebugBundle(t *testing.T) {
	mustClean(t)
	projectID := "proj-debug-bundle-" + newID()
	jobID := seedJob(t, projectID)
	runID, _ := seedRun(t, jobID)

	tgt := newTargeter("GET", "/v1/runs/"+runID+"/debug-bundle", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "run-debug-bundle", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.70)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "run-debug-bundle", tgt)
		assertSuccessRate(t, m, 0.60)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "run-debug-bundle", tgt)
		assertSuccessRate(t, m, 0.50)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_SetDebugMode(t *testing.T) {
	mustClean(t)
	projectID := "proj-set-debug-mode-" + newID()
	jobID := seedJob(t, projectID)
	runID, _ := seedRun(t, jobID)

	tgt := newTargeter("POST", "/v1/runs/"+runID+"/debug", func() []byte {
		return []byte(`{"debug_mode":true}`)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "run-set-debug-mode", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "run-set-debug-mode", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "run-set-debug-mode", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_Lineage(t *testing.T) {
	mustClean(t)
	projectID := "proj-run-lineage-" + newID()
	jobID := seedJob(t, projectID)
	runID, _ := seedRun(t, jobID)

	tgt := newTargeter("GET", "/v1/runs/"+runID+"/lineage", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "run-lineage", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "run-lineage", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "run-lineage", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestRuns_BulkDLQReplay(t *testing.T) {
	mustClean(t)
	projectID := "proj-bulk-dlq-replay-" + newID()
	jobID := seedJob(t, projectID)
	ctx := context.Background()

	deadLetterRunIDs := make([]string, 500)
	for i := range 500 {
		runID, _ := seedRun(t, jobID)
		if err := testStore.UpdateRunStatus(ctx, runID, domain.StatusQueued, domain.StatusDequeued, map[string]any{"started_at": time.Now().UTC()}); err != nil {
			t.Fatalf("dequeue run %s: %v", runID, err)
		}
		if err := testStore.UpdateRunStatus(ctx, runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{}); err != nil {
			t.Fatalf("execute run %s: %v", runID, err)
		}
		if err := testStore.UpdateRunStatus(ctx, runID, domain.StatusExecuting, domain.StatusDeadLetter, map[string]any{"finished_at": time.Now().UTC(), "error": "dead letter for load test"}); err != nil {
			t.Fatalf("dead-letter run %s: %v", runID, err)
		}
		deadLetterRunIDs[i] = runID
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := int(idx.Add(1)-1) * 5
		if i+5 > len(deadLetterRunIDs) {
			i %= len(deadLetterRunIDs) - 4
		}
		var runIDsJSON strings.Builder
		for j := range 5 {
			if j > 0 {
				runIDsJSON.WriteString(",")
			}
			_, _ = fmt.Fprintf(&runIDsJSON, `"%s"`, deadLetterRunIDs[i+j])
		}
		tgt.Method = "POST"
		tgt.URL = baseURL + "/v1/runs/bulk-dlq-replay"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
		}
		tgt.Body = fmt.Appendf(nil, `{"run_ids":[%s]}`, runIDsJSON.String())
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "bulk-dlq-replay", tgt, withRate(20))
		assertSuccessRate(t, m, 0.70)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "bulk-dlq-replay", tgt, withWorkers(10))
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "bulk-dlq-replay", tgt)
		assertNoServerErrors(t, m)
	})
}
