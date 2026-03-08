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

	"orchestrator/internal/domain"
)

func mustCleanAdv(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if err := testEnv.DB.CleanTables(ctx); err != nil {
		t.Fatalf("clean tables: %v", err)
	}
}

func advAuthedReq(method, path, body string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("X-Internal-Secret", "test-secret")
	r.Header.Set("Content-Type", "application/json")
	return r
}

func advDoReq(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, advAuthedReq(method, path, body))
	return w
}

func advDecodeMap(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func advDecodeSlice(t *testing.T, w *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
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
	if w.Code != http.StatusCreated {
		t.Fatalf("create job status = %d; body = %s", w.Code, w.Body.String())
	}
	return advDecodeMap(t, w)
}

func advTriggerRun(t *testing.T, jobID string) map[string]any {
	t.Helper()
	w := advDoReq(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger status = %d; body = %s", w.Code, w.Body.String())
	}
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

	if got := int(job["version"].(float64)); got != 1 {
		t.Fatalf("initial version = %d, want 1", got)
	}

	w1 := advDoReq(t, http.MethodPatch, "/v1/jobs/"+jobID, `{"name":"Versioned Job V2"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("first patch status = %d; body = %s", w1.Code, w1.Body.String())
	}
	if got := int(advDecodeMap(t, w1)["version"].(float64)); got != 2 {
		t.Fatalf("version after first patch = %d, want 2", got)
	}

	gw := advDoReq(t, http.MethodGet, "/v1/jobs/"+jobID, "")
	if gw.Code != http.StatusOK {
		t.Fatalf("get job status = %d; body = %s", gw.Code, gw.Body.String())
	}
	if got := int(advDecodeMap(t, gw)["version"].(float64)); got != 2 {
		t.Fatalf("version after get = %d, want 2", got)
	}

	w2 := advDoReq(t, http.MethodPatch, "/v1/jobs/"+jobID, `{"name":"Versioned Job V3"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("second patch status = %d; body = %s", w2.Code, w2.Body.String())
	}
	if got := int(advDecodeMap(t, w2)["version"].(float64)); got != 3 {
		t.Fatalf("version after second patch = %d, want 3", got)
	}
}

func TestE2E_JobVersioning_VersionHistory(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-ver-history")
	job := advCreateJob(t, projectID, advUnique("job-ver-history"), "History Base", "", 0)
	jobID := job["id"].(string)

	if w := advDoReq(t, http.MethodPatch, "/v1/jobs/"+jobID, `{"name":"History V2"}`); w.Code != http.StatusOK {
		t.Fatalf("first patch status = %d; body = %s", w.Code, w.Body.String())
	}
	if w := advDoReq(t, http.MethodPatch, "/v1/jobs/"+jobID, `{"name":"History V3"}`); w.Code != http.StatusOK {
		t.Fatalf("second patch status = %d; body = %s", w.Code, w.Body.String())
	}

	vw := advDoReq(t, http.MethodGet, "/v1/jobs/"+jobID+"/versions", "")
	if vw.Code != http.StatusOK {
		t.Fatalf("versions status = %d; body = %s", vw.Code, vw.Body.String())
	}
	versions := advDecodeSlice(t, vw)
	if len(versions) != 2 {
		t.Fatalf("versions len = %d, want 2", len(versions))
	}
	if got := int(versions[0]["version"].(float64)); got != 2 {
		t.Fatalf("first snapshot version = %d, want 2", got)
	}
	if got := versions[0]["name"].(string); got != "History V2" {
		t.Fatalf("first snapshot name = %q, want %q", got, "History V2")
	}
	if got := int(versions[1]["version"].(float64)); got != 1 {
		t.Fatalf("second snapshot version = %d, want 1", got)
	}
	if got := versions[1]["name"].(string); got != "History Base" {
		t.Fatalf("second snapshot name = %q, want %q", got, "History Base")
	}
}

func TestE2E_JobVersioning_RunStamped(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-run-stamped")
	job := advCreateJob(t, projectID, advUnique("job-run-stamped"), "Stamped Job", "", 0)
	jobID := job["id"].(string)

	trigger1 := advTriggerRun(t, jobID)
	run1ID := trigger1["id"].(string)
	r1 := advDoReq(t, http.MethodGet, "/v1/runs/"+run1ID, "")
	if r1.Code != http.StatusOK {
		t.Fatalf("get run1 status = %d; body = %s", r1.Code, r1.Body.String())
	}
	if got := int(advDecodeMap(t, r1)["job_version"].(float64)); got != 1 {
		t.Fatalf("run1 job_version = %d, want 1", got)
	}

	if w := advDoReq(t, http.MethodPatch, "/v1/jobs/"+jobID, `{"name":"Stamped Job V2"}`); w.Code != http.StatusOK {
		t.Fatalf("patch status = %d; body = %s", w.Code, w.Body.String())
	}

	trigger2 := advTriggerRun(t, jobID)
	run2ID := trigger2["id"].(string)
	r2 := advDoReq(t, http.MethodGet, "/v1/runs/"+run2ID, "")
	if r2.Code != http.StatusOK {
		t.Fatalf("get run2 status = %d; body = %s", r2.Code, r2.Body.String())
	}
	if got := int(advDecodeMap(t, r2)["job_version"].(float64)); got != 2 {
		t.Fatalf("run2 job_version = %d, want 2", got)
	}
}

func TestE2E_APIKey_CreateAndList(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-api-key")

	cw := advDoReq(t, http.MethodPost, "/v1/api-keys/", fmt.Sprintf(`{"project_id":"%s","name":"My Key"}`, projectID))
	if cw.Code != http.StatusCreated {
		t.Fatalf("create api key status = %d; body = %s", cw.Code, cw.Body.String())
	}
	created := advDecodeMap(t, cw)
	keyID := created["id"].(string)
	rawKey := created["key"].(string)
	if keyID == "" || !strings.HasPrefix(rawKey, "orc_") {
		t.Fatalf("unexpected key creation response: %#v", created)
	}
	if _, ok := created["key_prefix"]; !ok {
		t.Fatalf("missing key_prefix in response: %#v", created)
	}

	lw := advDoReq(t, http.MethodGet, "/v1/api-keys/?project_id="+projectID, "")
	if lw.Code != http.StatusOK {
		t.Fatalf("list api keys status = %d; body = %s", lw.Code, lw.Body.String())
	}
	keys := advDecodeSlice(t, lw)
	if len(keys) != 1 {
		t.Fatalf("api keys len = %d, want 1", len(keys))
	}
	if keys[0]["id"].(string) != keyID {
		t.Fatalf("listed key id = %q, want %q", keys[0]["id"].(string), keyID)
	}
	if _, ok := keys[0]["key"]; ok {
		t.Fatalf("list response should not include raw key: %#v", keys[0])
	}
}

func TestE2E_APIKey_Authenticate(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-api-auth")
	advCreateJob(t, projectID, advUnique("job-api-auth"), "API Auth Job", "", 0)

	cw := advDoReq(t, http.MethodPost, "/v1/api-keys/", fmt.Sprintf(`{"project_id":"%s","name":"Auth Key"}`, projectID))
	if cw.Code != http.StatusCreated {
		t.Fatalf("create api key status = %d; body = %s", cw.Code, cw.Body.String())
	}
	rawKey := advDecodeMap(t, cw)["key"].(string)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/?project_id="+projectID, nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("api key auth status = %d; body = %s", w.Code, w.Body.String())
	}
}

func TestE2E_APIKey_Revoke(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-api-revoke")

	cw := advDoReq(t, http.MethodPost, "/v1/api-keys/", fmt.Sprintf(`{"project_id":"%s","name":"Revoke Key"}`, projectID))
	if cw.Code != http.StatusCreated {
		t.Fatalf("create api key status = %d; body = %s", cw.Code, cw.Body.String())
	}
	created := advDecodeMap(t, cw)
	keyID := created["id"].(string)
	rawKey := created["key"].(string)

	rw := advDoReq(t, http.MethodDelete, "/v1/api-keys/"+keyID, "")
	if rw.Code != http.StatusOK {
		t.Fatalf("revoke api key status = %d; body = %s", rw.Code, rw.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/?project_id="+projectID, nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key auth status = %d, want 401; body = %s", w.Code, w.Body.String())
	}
}

func TestE2E_BulkTrigger(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-bulk-trigger")
	job := advCreateJob(t, projectID, advUnique("job-bulk-trigger"), "Bulk Trigger Job", "", 0)
	jobID := job["id"].(string)

	body := `{"items":[{"payload":{"i":1},"priority":0},{"payload":{"i":2},"priority":1},{"payload":{"i":3},"priority":2},{"payload":{"i":4},"priority":3},{"payload":{"i":5},"priority":4}]}`
	w := advDoReq(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger/bulk", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("bulk trigger status = %d; body = %s", w.Code, w.Body.String())
	}
	resp := advDecodeMap(t, w)
	resultsAny, ok := resp["results"].([]any)
	if !ok || len(resultsAny) != 5 {
		t.Fatalf("bulk results len mismatch: %#v", resp)
	}
	runIDs := map[string]bool{}
	runTokens := map[string]bool{}
	for i, raw := range resultsAny {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("result[%d] invalid type: %T", i, raw)
		}
		rid := item["id"].(string)
		rtok := item["run_token"].(string)
		if rid == "" || rtok == "" {
			t.Fatalf("result[%d] missing id/token: %#v", i, item)
		}
		runIDs[rid] = true
		runTokens[rtok] = true
	}
	if len(runIDs) != 5 || len(runTokens) != 5 {
		t.Fatalf("expected unique run ids/tokens, got ids=%d tokens=%d", len(runIDs), len(runTokens))
	}
}

func TestE2E_BulkTrigger_Validation(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-bulk-trigger-validation")
	job := advCreateJob(t, projectID, advUnique("job-bulk-trigger-validation"), "Bulk Validation Job", "", 0)
	jobID := job["id"].(string)

	w := advDoReq(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger/bulk", `{"items":[]}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bulk validation status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
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
	if bw.Code != http.StatusOK {
		t.Fatalf("bulk cancel status = %d; body = %s", bw.Code, bw.Body.String())
	}
	resp := advDecodeMap(t, bw)
	if int(resp["canceled"].(float64)) != 3 || int(resp["failed"].(float64)) != 0 {
		t.Fatalf("unexpected bulk cancel counters: %#v", resp)
	}

	for _, runID := range runIDs {
		rw := advDoReq(t, http.MethodGet, "/v1/runs/"+runID, "")
		if rw.Code != http.StatusOK {
			t.Fatalf("get run status = %d; body = %s", rw.Code, rw.Body.String())
		}
		if got := advDecodeMap(t, rw)["status"].(string); got != string(domain.StatusCanceled) {
			t.Fatalf("run %s status = %q, want %q", runID, got, domain.StatusCanceled)
		}
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
	if cw.Code != http.StatusOK {
		t.Fatalf("direct cancel status = %d; body = %s", cw.Code, cw.Body.String())
	}

	body := fmt.Sprintf(`{"run_ids":["%s","%s"]}`, run1, run2)
	bw := advDoReq(t, http.MethodPost, "/v1/runs/bulk-cancel", body)
	if bw.Code != http.StatusOK {
		t.Fatalf("bulk cancel status = %d; body = %s", bw.Code, bw.Body.String())
	}
	resp := advDecodeMap(t, bw)
	if int(resp["canceled"].(float64)) != 1 || int(resp["failed"].(float64)) != 1 {
		t.Fatalf("unexpected bulk cancel counters: %#v", resp)
	}
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
	runToken := parentRun["run_token"].(string)

	spawnBody := fmt.Sprintf(`{"job_slug":"%s","project_id":"%s","payload":{"child":true}}`, childSlug, projectID)
	sw := advDoSDKReq(t, http.MethodPost, "/sdk/v1/runs/"+parentRunID+"/spawn", runToken, spawnBody)
	if sw.Code != http.StatusCreated {
		t.Fatalf("spawn status = %d; body = %s", sw.Code, sw.Body.String())
	}
	childRunID := advDecodeMap(t, sw)["id"].(string)

	cw := advDoReq(t, http.MethodDelete, "/v1/runs/"+parentRunID, "")
	if cw.Code != http.StatusOK {
		t.Fatalf("cancel parent status = %d; body = %s", cw.Code, cw.Body.String())
	}

	pw := advDoReq(t, http.MethodGet, "/v1/runs/"+parentRunID, "")
	if pw.Code != http.StatusOK {
		t.Fatalf("get parent status = %d; body = %s", pw.Code, pw.Body.String())
	}
	if got := advDecodeMap(t, pw)["status"].(string); got != string(domain.StatusCanceled) {
		t.Fatalf("parent status = %q, want %q", got, domain.StatusCanceled)
	}

	chw := advDoReq(t, http.MethodGet, "/v1/runs/"+childRunID, "")
	if chw.Code != http.StatusOK {
		t.Fatalf("get child status = %d; body = %s", chw.Code, chw.Body.String())
	}
	if got := advDecodeMap(t, chw)["status"].(string); got != string(domain.StatusCanceled) {
		t.Fatalf("child status = %q, want %q", got, domain.StatusCanceled)
	}
}

func TestE2E_EventLogging(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-events")
	job := advCreateJob(t, projectID, advUnique("job-events"), "Events Job", "", 0)
	trigger := advTriggerRun(t, job["id"].(string))
	runID := trigger["id"].(string)
	runToken := trigger["run_token"].(string)

	lw := advDoSDKReq(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/log", runToken, `{"message":"hello event","level":"info"}`)
	if lw.Code != http.StatusCreated {
		t.Fatalf("log status = %d; body = %s", lw.Code, lw.Body.String())
	}

	ew := advDoReq(t, http.MethodGet, "/v1/runs/"+runID+"/events", "")
	if ew.Code != http.StatusOK {
		t.Fatalf("list events status = %d; body = %s", ew.Code, ew.Body.String())
	}
	events := advDecodeSlice(t, ew)
	found := false
	for _, evt := range events {
		if evt["message"] == "hello event" && evt["level"] == "info" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected logged event not found: %#v", events)
	}
}

func TestE2E_EventFiltering(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-events-filter")
	job := advCreateJob(t, projectID, advUnique("job-events-filter"), "Events Filter Job", "", 0)
	trigger := advTriggerRun(t, job["id"].(string))
	runID := trigger["id"].(string)
	runToken := trigger["run_token"].(string)

	if w := advDoSDKReq(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/log", runToken, `{"message":"all good","level":"info"}`); w.Code != http.StatusCreated {
		t.Fatalf("log info status = %d; body = %s", w.Code, w.Body.String())
	}
	if w := advDoSDKReq(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/log", runToken, `{"message":"something broke","level":"error"}`); w.Code != http.StatusCreated {
		t.Fatalf("log error status = %d; body = %s", w.Code, w.Body.String())
	}

	ew := advDoReq(t, http.MethodGet, "/v1/runs/"+runID+"/events?level=error", "")
	if ew.Code != http.StatusOK {
		t.Fatalf("list filtered events status = %d; body = %s", ew.Code, ew.Body.String())
	}
	events := advDecodeSlice(t, ew)
	if len(events) == 0 {
		t.Fatalf("expected at least one filtered event, got none")
	}
	for _, evt := range events {
		if evt["level"] != "error" {
			t.Fatalf("unexpected non-error event in filtered results: %#v", evt)
		}
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
	if rw.Code != http.StatusOK {
		t.Fatalf("get run status = %d; body = %s", rw.Code, rw.Body.String())
	}
	run := advDecodeMap(t, rw)
	expiresRaw, ok := run["expires_at"].(string)
	if !ok || expiresRaw == "" {
		t.Fatalf("missing expires_at in run: %#v", run)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresRaw)
	if err != nil {
		t.Fatalf("parse expires_at: %v", err)
	}
	want := start.Add(3600 * time.Second)
	delta := expiresAt.Sub(want)
	if delta < -5*time.Second || delta > 5*time.Second {
		t.Fatalf("expires_at delta = %v, want within 5s", delta)
	}
}

func TestE2E_CronJob(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-cron")
	cronExpr := "*/5 * * * *"
	cronJob := advCreateJob(t, projectID, advUnique("job-cron-enabled"), "Cron Enabled", cronExpr, 0)
	plainJob := advCreateJob(t, projectID, advUnique("job-cron-empty"), "No Cron", "", 0)

	lw := advDoReq(t, http.MethodGet, "/v1/jobs/?project_id="+projectID, "")
	if lw.Code != http.StatusOK {
		t.Fatalf("list jobs status = %d; body = %s", lw.Code, lw.Body.String())
	}
	jobs := advDecodeSlice(t, lw)
	if len(jobs) != 2 {
		t.Fatalf("jobs len = %d, want 2", len(jobs))
	}

	byID := map[string]map[string]any{}
	for _, item := range jobs {
		byID[item["id"].(string)] = item
	}
	if got := byID[cronJob["id"].(string)]["cron"].(string); got != cronExpr {
		t.Fatalf("cron job cron = %q, want %q", got, cronExpr)
	}
	if item, ok := byID[plainJob["id"].(string)]; ok {
		if v, present := item["cron"]; present && v != "" {
			t.Fatalf("expected empty cron for non-cron job, got %#v", v)
		}
	} else {
		t.Fatalf("plain job not found in list")
	}
}

func TestE2E_WebhookDeliveries(t *testing.T) {
	mustCleanAdv(t)
	w := advDoReq(t, http.MethodGet, "/v1/webhook-deliveries?project_id=proj-webhook-test", "")
	if w.Code != http.StatusOK {
		t.Fatalf("webhook deliveries status = %d; body = %s", w.Code, w.Body.String())
	}
	var deliveries []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&deliveries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func TestE2E_TriggerDisabledJob(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-disabled-job")
	job := advCreateJob(t, projectID, advUnique("job-disabled"), "Disabled Job", "", 0)
	jobID := job["id"].(string)

	dw := advDoReq(t, http.MethodDelete, "/v1/jobs/"+jobID, "")
	if dw.Code != http.StatusNoContent {
		t.Fatalf("delete job status = %d; body = %s", dw.Code, dw.Body.String())
	}

	tw := advDoReq(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{}`)
	if tw.Code != http.StatusBadRequest {
		t.Fatalf("trigger disabled job status = %d, want 400; body = %s", tw.Code, tw.Body.String())
	}
	if !strings.Contains(tw.Body.String(), "job is disabled") {
		t.Fatalf("expected disabled job error, got %s", tw.Body.String())
	}
}

func TestE2E_GetNonexistentRun(t *testing.T) {
	mustCleanAdv(t)
	w := advDoReq(t, http.MethodGet, "/v1/runs/nonexistent-id", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("get nonexistent run status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestE2E_CancelTerminalRun(t *testing.T) {
	mustCleanAdv(t)
	projectID := advUnique("proj-cancel-terminal")
	job := advCreateJob(t, projectID, advUnique("job-cancel-terminal"), "Cancel Terminal Job", "", 0)
	trigger := advTriggerRun(t, job["id"].(string))
	runID := trigger["id"].(string)
	runToken := trigger["run_token"].(string)

	ctx := context.Background()
	if err := testStore.UpdateRunStatus(ctx, runID, domain.StatusQueued, domain.StatusDequeued, map[string]any{"started_at": time.Now()}); err != nil {
		t.Fatalf("set run dequeued: %v", err)
	}
	if err := testStore.UpdateRunStatus(ctx, runID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{"started_at": time.Now()}); err != nil {
		t.Fatalf("set run executing: %v", err)
	}

	cw := advDoSDKReq(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/complete", runToken, `{}`)
	if cw.Code != http.StatusOK {
		t.Fatalf("complete run status = %d; body = %s", cw.Code, cw.Body.String())
	}

	xw := advDoReq(t, http.MethodDelete, "/v1/runs/"+runID, "")
	if xw.Code != http.StatusBadRequest {
		t.Fatalf("cancel terminal run status = %d, want 400; body = %s", xw.Code, xw.Body.String())
	}
	if !strings.Contains(xw.Body.String(), "run already in terminal state") {
		t.Fatalf("expected terminal state error, got %s", xw.Body.String())
	}
}
