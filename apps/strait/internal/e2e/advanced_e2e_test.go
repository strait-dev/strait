//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func mustCleanAdv(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, testEnv.
		DB.CleanTables(
		ctx))

}

func advAuthedReq(method, path, body string, projectID ...string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("X-Internal-Secret", "test-secret-value")
	r.Header.Set("Content-Type", "application/json")
	if len(projectID) > 0 && projectID[0] != "" {
		r.Header.Set("X-Project-Id", projectID[0])
	}
	return r
}

func advDoReq(t *testing.T, method, path, body string, projectID ...string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, advAuthedReq(method, path, body, projectID...))
	return w
}

func advDecodeMap(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp map[string]any
	require.NoError(t, json.
		NewDecoder(w.Body).
		Decode(&resp))

	return resp
}

func advDecodeSlice(t *testing.T, w *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.
		NewDecoder(w.Body).
		Decode(&envelope))

	return envelope.Data
}

func advUnique(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func advCreateJob(t *testing.T, projectID, slug, name, cron string, runTTLSecs int) map[string]any {
	t.Helper()
	body := fmt.Sprintf(`{"project_id":"%s","name":"%s","slug":"%s","endpoint_url":"https://example.com/callback","max_attempts":3,"timeout_secs":300`, projectID, name, slug)
	if cron != "" {
		body += fmt.Sprintf(`,"cron":"%s"`, cron)
	}
	if runTTLSecs > 0 {
		body += fmt.Sprintf(`,"run_ttl_secs":%d`, runTTLSecs)
	}
	body += "}"

	w := advDoReq(t, http.MethodPost, "/v1/jobs/", body)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	return advDecodeMap(t, w)
}

func advTriggerRun(t *testing.T, jobID string) map[string]any {
	t.Helper()
	w := advDoReq(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	return advDecodeMap(t, w)
}

func advDoSDKReq(t *testing.T, method, path, runToken, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Authorization", "Bearer "+runToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, req)
	return w
}

func TestE2E_JobVersioning_IncrementOnUpdate(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-ver-inc")
	job := advCreateJob(t, projectID, advUnique("job-ver-inc"), "Versioned Job", "", 0)
	jobID := job["id"].(string)
	require.EqualValues(t, 1, int(job["version"].(float64)))

	w1 := advDoReq(t, http.MethodPatch, "/v1/jobs/"+jobID, `{"name":"Versioned Job V2"}`)
	require.Equal(t, http.
		StatusOK,
		w1.Code)
	require.EqualValues(t, 2, int(advDecodeMap(t, w1)["version"].(float64)))

	gw := advDoReq(t, http.MethodGet, "/v1/jobs/"+jobID, "")
	require.Equal(t, http.
		StatusOK,
		gw.Code)
	require.EqualValues(t, 2, int(advDecodeMap(t, gw)["version"].(float64)))

	w2 := advDoReq(t, http.MethodPatch, "/v1/jobs/"+jobID, `{"name":"Versioned Job V3"}`)
	require.Equal(t, http.
		StatusOK,
		w2.Code)
	require.EqualValues(t, 3, int(advDecodeMap(t, w2)["version"].(float64)))

}

func TestE2E_JobVersioning_VersionHistory(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-ver-history")
	job := advCreateJob(t, projectID, advUnique("job-ver-history"), "History Base", "", 0)
	jobID := job["id"].(string)

	if w := advDoReq(t, http.MethodPatch, "/v1/jobs/"+jobID, `{"name":"History V2"}`); w.Code != http.StatusOK {
		require.Failf(t, "test failure",

			"first patch status = %d; body = %s", w.Code, w.Body.String())
	}
	if w := advDoReq(t, http.MethodPatch, "/v1/jobs/"+jobID, `{"name":"History V3"}`); w.Code != http.StatusOK {
		require.Failf(t, "test failure",

			"second patch status = %d; body = %s", w.Code, w.Body.String())
	}

	vw := advDoReq(t, http.MethodGet, "/v1/jobs/"+jobID+"/versions", "")
	require.Equal(t, http.
		StatusOK,
		vw.Code)

	versions := advDecodeSlice(t, vw)
	require.Len(t, versions,

		2)
	require.EqualValues(t, 2, int(versions[0]["version"].(float64)))
	require.Equal(t, "History V2",

		versions[0]["name"].(string),
	)
	require.EqualValues(t, 1, int(versions[1]["version"].(float64)))
	require.Equal(t, "History Base",

		versions[1]["name"].(string))

}

func TestE2E_JobVersioning_RunStamped(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-run-stamped")
	job := advCreateJob(t, projectID, advUnique("job-run-stamped"), "Stamped Job", "", 0)
	jobID := job["id"].(string)

	trigger1 := advTriggerRun(t, jobID)
	run1ID := trigger1["id"].(string)
	r1 := advDoReq(t, http.MethodGet, "/v1/runs/"+run1ID, "")
	require.Equal(t, http.
		StatusOK,
		r1.Code)
	require.EqualValues(t, 1, int(advDecodeMap(t, r1)["job_version"].(float64)))

	if w := advDoReq(t, http.MethodPatch, "/v1/jobs/"+jobID, `{"name":"Stamped Job V2"}`); w.Code != http.StatusOK {
		require.Failf(t, "test failure",

			"patch status = %d; body = %s", w.Code, w.Body.String())
	}

	trigger2 := advTriggerRun(t, jobID)
	run2ID := trigger2["id"].(string)
	r2 := advDoReq(t, http.MethodGet, "/v1/runs/"+run2ID, "")
	require.Equal(t, http.
		StatusOK,
		r2.Code)
	require.EqualValues(t, 2, int(advDecodeMap(t, r2)["job_version"].(float64)))

}

func TestE2E_APIKey_CreateAndList(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-api-key")

	cw := advDoReq(t, http.MethodPost, "/v1/api-keys/", fmt.Sprintf(`{"project_id":"%s","name":"My Key","scopes":["%s"],"expires_in_days":30}`, projectID, domain.ScopeJobsRead))
	require.Equal(t, http.
		StatusCreated,
		cw.Code,
	)

	created := advDecodeMap(t, cw)
	keyID := created["id"].(string)
	rawKey := created["key"].(string)
	require.False(t, keyID ==
		"" ||
		!strings.HasPrefix(rawKey,
			"strait_",
		))

	if _, ok := created["key_prefix"]; !ok {
		require.Failf(t, "test failure",

			"missing key_prefix in response: %#v", created)
	}

	lw := advDoReq(t, http.MethodGet, "/v1/api-keys/", "", projectID)
	require.Equal(t, http.
		StatusOK,
		lw.Code)

	keys := advDecodeSlice(t, lw)
	require.Len(t, keys,
		1,
	)
	require.Equal(t, keyID,

		keys[0]["id"].(string))

	if _, ok := keys[0]["key"]; ok {
		require.Failf(t, "test failure",

			"list response should not include raw key: %#v", keys[0])
	}
}

func TestE2E_APIKey_Authenticate(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-api-auth")
	advCreateJob(t, projectID, advUnique("job-api-auth"), "API Auth Job", "", 0)

	cw := advDoReq(t, http.MethodPost, "/v1/api-keys/", fmt.Sprintf(`{"project_id":"%s","name":"Auth Key","scopes":["%s"],"expires_in_days":30}`, projectID, domain.ScopeJobsRead))
	require.Equal(t, http.
		StatusCreated,
		cw.Code,
	)

	rawKey := advDecodeMap(t, cw)["key"].(string)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusOK,
		w.Code)

}

func TestE2E_APIKey_Revoke(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-api-revoke")

	cw := advDoReq(t, http.MethodPost, "/v1/api-keys/", fmt.Sprintf(`{"project_id":"%s","name":"Revoke Key","scopes":["%s"],"expires_in_days":30}`, projectID, domain.ScopeJobsRead))
	require.Equal(t, http.
		StatusCreated,
		cw.Code,
	)

	created := advDecodeMap(t, cw)
	keyID := created["id"].(string)
	rawKey := created["key"].(string)

	rw := advDoReq(t, http.MethodDelete, "/v1/api-keys/"+keyID, "")
	require.Equal(t, http.
		StatusOK,
		rw.Code)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusUnauthorized,

		w.Code)

}

func TestE2E_BulkTrigger(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-bulk-trigger")
	job := advCreateJob(t, projectID, advUnique("job-bulk-trigger"), "Bulk Trigger Job", "", 0)
	jobID := job["id"].(string)

	body := `{"items":[{"payload":{"i":1},"priority":0},{"payload":{"i":2},"priority":1},{"payload":{"i":3},"priority":2},{"payload":{"i":4},"priority":3},{"payload":{"i":5},"priority":4}]}`
	w := advDoReq(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger/bulk", body)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	resp := advDecodeMap(t, w)
	resultsAny, ok := resp["results"].([]any)
	require.False(t, !ok ||
		len(resultsAny) !=
			5)

	runIDs := map[string]bool{}
	for i, raw := range resultsAny {
		item, ok := raw.(map[string]any)
		require.True(t, ok)

		rid, ok := item["id"].(string)
		require.False(t, !ok ||
			rid ==
				"")

		status, ok := item["status"].(string)
		require.False(t, !ok ||
			status !=
				string(domain.
					StatusQueued,
				))

		if _, ok := item["run_token"]; ok {
			require.Failf(t, "test failure",

				"result[%d] must not expose SDK run_token: %#v", i, item)
		}
		runIDs[rid] = true
	}
	require.Len(t, runIDs,

		5)

}

func TestE2E_BulkTrigger_Validation(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-bulk-trigger-validation")
	job := advCreateJob(t, projectID, advUnique("job-bulk-trigger-validation"), "Bulk Validation Job", "", 0)
	jobID := job["id"].(string)

	w := advDoReq(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger/bulk", `{"items":[]}`)
	require.Equal(t, http.
		StatusUnprocessableEntity,

		w.Code)

}

func TestE2E_BulkCancel(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-bulk-cancel")
	job := advCreateJob(t, projectID, advUnique("job-bulk-cancel"), "Bulk Cancel Job", "", 0)
	jobID := job["id"].(string)

	runIDs := make([]string, 0, 3)
	for range 3 {
		runIDs = append(runIDs, advTriggerRun(t, jobID)["id"].(string))
	}
	body := fmt.Sprintf(`{"run_ids":["%s","%s","%s"]}`, runIDs[0], runIDs[1], runIDs[2])
	bw := advDoReq(t, http.MethodPost, "/v1/runs/bulk-cancel", body)
	require.Equal(t, http.
		StatusOK,
		bw.Code)

	resp := advDecodeMap(t, bw)
	require.False(t, int(
		resp["canceled"].(float64)) != 3 || int(resp["failed"].(float64)) != 0)

	for _, runID := range runIDs {
		rw := advDoReq(t, http.MethodGet, "/v1/runs/"+runID, "")
		require.Equal(t, http.
			StatusOK,
			rw.Code)
		require.Equal(t, string(domain.
			StatusCanceled,
		), advDecodeMap(t, rw)["status"].(string))

	}
}

func TestE2E_BulkCancel_PartialFailure(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-bulk-cancel-partial")
	job := advCreateJob(t, projectID, advUnique("job-bulk-cancel-partial"), "Bulk Cancel Partial Job", "", 0)
	jobID := job["id"].(string)

	run1 := advTriggerRun(t, jobID)["id"].(string)
	run2 := advTriggerRun(t, jobID)["id"].(string)

	cw := advDoReq(t, http.MethodDelete, "/v1/runs/"+run1, "")
	require.Equal(t, http.
		StatusOK,
		cw.Code)

	body := fmt.Sprintf(`{"run_ids":["%s","%s"]}`, run1, run2)
	bw := advDoReq(t, http.MethodPost, "/v1/runs/bulk-cancel", body)
	require.Equal(t, http.
		StatusOK,
		bw.Code)

	resp := advDecodeMap(t, bw)
	require.False(t, int(
		resp["canceled"].(float64)) != 1 || int(resp["failed"].(float64)) != 1)

}

func TestE2E_CancelPropagation(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-cancel-prop")
	parentSlug := advUnique("job-parent")
	childSlug := advUnique("job-child")
	parentJob := advCreateJob(t, projectID, parentSlug, "Parent Job", "", 0)
	_ = advCreateJob(t, projectID, childSlug, "Child Job", "", 0)

	parentRun := advTriggerRun(t, parentJob["id"].(string))
	parentRunID := parentRun["id"].(string)
	runToken := makeE2ERunToken(t, parentRunID)
	activateE2ERun(t, parentRunID)

	spawnBody := fmt.Sprintf(`{"job_slug":"%s","project_id":"%s","payload":{"child":true}}`, childSlug, projectID)
	sw := advDoSDKReq(t, http.MethodPost, "/sdk/v1/runs/"+parentRunID+"/spawn", runToken, spawnBody)
	require.Equal(t, http.
		StatusCreated,
		sw.Code,
	)

	childRunID := advDecodeMap(t, sw)["id"].(string)

	cw := advDoReq(t, http.MethodDelete, "/v1/runs/"+parentRunID, "")
	require.Equal(t, http.
		StatusOK,
		cw.Code)

	pw := advDoReq(t, http.MethodGet, "/v1/runs/"+parentRunID, "")
	require.Equal(t, http.
		StatusOK,
		pw.Code)
	require.Equal(t, string(domain.
		StatusCanceled,
	), advDecodeMap(t, pw)["status"].(string))

	chw := advDoReq(t, http.MethodGet, "/v1/runs/"+childRunID, "")
	require.Equal(t, http.
		StatusOK,
		chw.Code)
	require.Equal(t, string(domain.
		StatusCanceled,
	), advDecodeMap(t, chw)["status"].(string))

}

func TestE2E_EventLogging(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-events")
	job := advCreateJob(t, projectID, advUnique("job-events"), "Events Job", "", 0)
	trigger := advTriggerRun(t, job["id"].(string))
	runID := trigger["id"].(string)
	runToken := makeE2ERunToken(t, runID)
	activateE2ERun(t, runID)

	lw := advDoSDKReq(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/log", runToken, `{"message":"hello event","level":"info"}`)
	require.Equal(t, http.
		StatusCreated,
		lw.Code,
	)

	ew := advDoReq(t, http.MethodGet, "/v1/runs/"+runID+"/events", "")
	require.Equal(t, http.
		StatusOK,
		ew.Code)

	events := advDecodeSlice(t, ew)
	found := false
	for _, evt := range events {
		if evt["message"] == "hello event" && evt["level"] == "info" {
			found = true
			break
		}
	}
	require.True(t, found)

}

func TestE2E_EventFiltering(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-events-filter")
	job := advCreateJob(t, projectID, advUnique("job-events-filter"), "Events Filter Job", "", 0)
	trigger := advTriggerRun(t, job["id"].(string))
	runID := trigger["id"].(string)
	runToken := makeE2ERunToken(t, runID)
	activateE2ERun(t, runID)

	if w := advDoSDKReq(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/log", runToken, `{"message":"all good","level":"info"}`); w.Code != http.StatusCreated {
		require.Failf(t, "test failure",

			"log info status = %d; body = %s", w.Code, w.Body.String())
	}
	if w := advDoSDKReq(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/log", runToken, `{"message":"something broke","level":"error"}`); w.Code != http.StatusCreated {
		require.Failf(t, "test failure",

			"log error status = %d; body = %s", w.Code, w.Body.String())
	}

	ew := advDoReq(t, http.MethodGet, "/v1/runs/"+runID+"/events?level=error", "")
	require.Equal(t, http.
		StatusOK,
		ew.Code)

	events := advDecodeSlice(t, ew)
	require.NotEmpty(t, events)

	for _, evt := range events {
		require.Equal(t, "error",

			evt["level"])

	}
}

func TestE2E_RunTTL(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-run-ttl")
	job := advCreateJob(t, projectID, advUnique("job-run-ttl"), "Run TTL Job", "", 3600)
	jobID := job["id"].(string)

	start := time.Now()
	trigger := advTriggerRun(t, jobID)
	runID := trigger["id"].(string)

	rw := advDoReq(t, http.MethodGet, "/v1/runs/"+runID, "")
	require.Equal(t, http.
		StatusOK,
		rw.Code)

	run := advDecodeMap(t, rw)
	expiresRaw, ok := run["expires_at"].(string)
	require.False(t, !ok ||
		expiresRaw ==
			"")

	expiresAt, err := time.Parse(time.RFC3339Nano, expiresRaw)
	require.NoError(t, err)

	want := start.Add(3600 * time.Second)
	delta := expiresAt.Sub(want)
	require.False(t, delta <
		-5*time.
			Second ||
		delta > 5*time.Second,
	)

}

func TestE2E_CronJob(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-cron")
	cronExpr := "*/5 * * * *"
	cronJob := advCreateJob(t, projectID, advUnique("job-cron-enabled"), "Cron Enabled", cronExpr, 0)
	plainJob := advCreateJob(t, projectID, advUnique("job-cron-empty"), "No Cron", "", 0)

	lw := advDoReq(t, http.MethodGet, "/v1/jobs/", "", projectID)
	require.Equal(t, http.
		StatusOK,
		lw.Code)

	jobs := advDecodeSlice(t, lw)
	require.Len(t, jobs,
		2,
	)

	byID := map[string]map[string]any{}
	for _, item := range jobs {
		byID[item["id"].(string)] = item
	}
	require.Equal(t, cronExpr,

		byID[cronJob["id"].(string)]["cron"].(string))

	if item, ok := byID[plainJob["id"].(string)]; ok {
		if v, present := item["cron"]; present && v != "" {
			require.Failf(t, "test failure",

				"expected empty cron for non-cron job, got %#v", v)
		}
	} else {
		require.Failf(t, "test failure",

			"plain job not found in list")
	}
}

func TestE2E_WebhookDeliveries(t *testing.T) {
	mustCleanAdv(t)
	w := advDoReq(t, http.MethodGet, "/v1/webhook-deliveries", "", "proj-webhook-test")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	_ = advDecodeSlice(t, w)
}

func TestE2E_TriggerDisabledJob(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-disabled-job")
	job := advCreateJob(t, projectID, advUnique("job-disabled"), "Disabled Job", "", 0)
	jobID := job["id"].(string)

	dw := advDoReq(t, http.MethodPatch, "/v1/jobs/"+jobID, `{"enabled":false}`)
	require.Equal(t, http.
		StatusOK,
		dw.Code)

	tw := advDoReq(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{}`)
	require.Equal(t, http.
		StatusBadRequest,
		tw.
			Code)
	require.True(t, strings.Contains(tw.Body.String(), "job is disabled"))

}

func TestE2E_GetNonexistentRun(t *testing.T) {
	mustCleanAdv(t)
	w := advDoReq(t, http.MethodGet, "/v1/runs/nonexistent-id", "")
	require.Equal(t, http.
		StatusNotFound,
		w.Code,
	)

}

func TestE2E_CancelTerminalRun(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-cancel-terminal")
	job := advCreateJob(t, projectID, advUnique("job-cancel-terminal"), "Cancel Terminal Job", "", 0)
	trigger := advTriggerRun(t, job["id"].(string))
	runID := trigger["id"].(string)
	runToken := makeE2ERunToken(t, runID)

	ctx := context.Background()
	require.NoError(t, testStore.
		UpdateRunStatus(ctx, runID, domain.
			StatusQueued,

			domain.StatusDequeued,
			map[string]any{"started_at": time.Now()}))
	require.NoError(t, testStore.
		UpdateRunStatus(ctx, runID, domain.
			StatusDequeued,

			domain.StatusExecuting,
			map[string]any{"started_at": time.Now()}))

	cw := advDoSDKReq(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/complete", runToken, `{}`)
	require.Equal(t, http.
		StatusOK,
		cw.Code)

	xw := advDoReq(t, http.MethodDelete, "/v1/runs/"+runID, "")
	require.Equal(t, http.
		StatusBadRequest,
		xw.
			Code)
	require.True(t, strings.Contains(xw.Body.String(), "run already in terminal state"))

}
