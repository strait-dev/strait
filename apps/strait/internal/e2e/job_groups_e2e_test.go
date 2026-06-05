//go:build integration

package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestE2E_JobGroup_CreateAndGet(t *testing.T) {
	mustClean(t)

	projectID := "proj-group-create-" + newID()
	body := fmt.Sprintf(`{"project_id":"%s","name":"Core Jobs","slug":"core-jobs-%s","description":"Core pipelines"}`,
		projectID, newID())

	w := doRequest(t, http.MethodPost, "/v1/job-groups/", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create job group status = %d, body = %s", w.Code, w.Body.String())
	}
	created := mustDecodeObject(t, w)
	groupID := asString(t, created, "id")

	w = doRequest(t, http.MethodGet, "/v1/job-groups/"+groupID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get job group status = %d, body = %s", w.Code, w.Body.String())
	}
	fetched := mustDecodeObject(t, w)
	if asString(t, fetched, "id") != groupID {
		t.Fatalf("expected group id %s, got %s", groupID, asString(t, fetched, "id"))
	}
	if asString(t, fetched, "project_id") != projectID {
		t.Fatalf("expected project_id %s", projectID)
	}
}

func TestE2E_JobGroup_ListByProject(t *testing.T) {
	mustClean(t)

	projectID := "proj-group-list-" + newID()
	otherProjectID := "proj-group-list-other-" + newID()

	w := doRequest(t, http.MethodPost, "/v1/job-groups/", fmt.Sprintf(`{"project_id":"%s","name":"Alpha","slug":"alpha-%s"}`,
		projectID, newID()))
	if w.Code != http.StatusCreated {
		t.Fatalf("create alpha group status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodPost, "/v1/job-groups/", fmt.Sprintf(`{"project_id":"%s","name":"Beta","slug":"beta-%s"}`,
		projectID, newID()))
	if w.Code != http.StatusCreated {
		t.Fatalf("create beta group status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodPost, "/v1/job-groups/", fmt.Sprintf(`{"project_id":"%s","name":"Other","slug":"other-%s"}`,
		otherProjectID, newID()))
	if w.Code != http.StatusCreated {
		t.Fatalf("create other group status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/job-groups/", "", projectID)
	if w.Code != http.StatusOK {
		t.Fatalf("list job groups status = %d, body = %s", w.Code, w.Body.String())
	}
	groups := mustDecodeList(t, w)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
}

func TestE2E_JobGroup_Update(t *testing.T) {
	mustClean(t)

	projectID := "proj-group-update-" + newID()
	w := doRequest(t, http.MethodPost, "/v1/job-groups/", fmt.Sprintf(`{"project_id":"%s","name":"Old Name","slug":"old-%s"}`,
		projectID, newID()))
	if w.Code != http.StatusCreated {
		t.Fatalf("create job group status = %d, body = %s", w.Code, w.Body.String())
	}
	groupID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodPatch, "/v1/job-groups/"+groupID, `{"name":"New Name","description":"Updated description"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("update job group status = %d, body = %s", w.Code, w.Body.String())
	}
	updated := mustDecodeObject(t, w)
	if asString(t, updated, "name") != "New Name" {
		t.Fatalf("expected updated name, got %s", asString(t, updated, "name"))
	}
	if asString(t, updated, "description") != "Updated description" {
		t.Fatalf("expected updated description, got %s", asString(t, updated, "description"))
	}
}

func TestE2E_JobGroup_DeleteAndListJobs(t *testing.T) {
	mustClean(t)

	projectID := "proj-group-delete-" + newID()
	createGroup := doRequest(t, http.MethodPost, "/v1/job-groups/", fmt.Sprintf(`{"project_id":"%s","name":"To Delete","slug":"del-%s"}`,
		projectID, newID()))
	if createGroup.Code != http.StatusCreated {
		t.Fatalf("create job group status = %d, body = %s", createGroup.Code, createGroup.Body.String())
	}
	groupID := asString(t, mustDecodeObject(t, createGroup), "id")

	jobBody := fmt.Sprintf(`{"project_id":"%s","group_id":"%s","name":"Grouped Job","slug":"grouped-%s","endpoint_url":"https://example.com/grouped","max_attempts":3,"timeout_secs":60}`,
		projectID, groupID, newID())
	w := doRequest(t, http.MethodPost, "/v1/jobs/", jobBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create grouped job status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodDelete, "/v1/job-groups/"+groupID, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete job group status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/job-groups/"+groupID+"/jobs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list jobs by deleted group status = %d, body = %s", w.Code, w.Body.String())
	}
	jobs := mustDecodeList(t, w)
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs after group deletion, got %d", len(jobs))
	}
}

func TestE2E_JobGroup_ListJobsByGroup(t *testing.T) {
	mustClean(t)

	projectID := "proj-group-jobs-" + newID()
	createGroup := doRequest(t, http.MethodPost, "/v1/job-groups/", fmt.Sprintf(`{"project_id":"%s","name":"Ops","slug":"ops-%s"}`,
		projectID, newID()))
	if createGroup.Code != http.StatusCreated {
		t.Fatalf("create job group status = %d, body = %s", createGroup.Code, createGroup.Body.String())
	}
	groupID := asString(t, mustDecodeObject(t, createGroup), "id")

	jobBody1 := fmt.Sprintf(`{"project_id":"%s","group_id":"%s","name":"Grouped One","slug":"grouped-one-%s","endpoint_url":"https://example.com/grouped-one","max_attempts":3,"timeout_secs":60}`,
		projectID, groupID, newID())
	w := doRequest(t, http.MethodPost, "/v1/jobs/", jobBody1)
	if w.Code != http.StatusCreated {
		t.Fatalf("create grouped job 1 status = %d, body = %s", w.Code, w.Body.String())
	}

	jobBody2 := fmt.Sprintf(`{"project_id":"%s","group_id":"%s","name":"Grouped Two","slug":"grouped-two-%s","endpoint_url":"https://example.com/grouped-two","max_attempts":3,"timeout_secs":60}`,
		projectID, groupID, newID())
	w = doRequest(t, http.MethodPost, "/v1/jobs/", jobBody2)
	if w.Code != http.StatusCreated {
		t.Fatalf("create grouped job 2 status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/job-groups/"+groupID+"/jobs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list jobs by group status = %d, body = %s", w.Code, w.Body.String())
	}
	jobs := mustDecodeList(t, w)
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs in group, got %d", len(jobs))
	}
}

func TestE2E_JobDependency_CreateAndList(t *testing.T) {
	mustClean(t)

	projectID := "proj-dep-create-" + newID()
	upstream := createJob(t, projectID, "Upstream", "upstream-"+newID())
	downstream := createJob(t, projectID, "Downstream", "downstream-"+newID())

	depBody := fmt.Sprintf(`{"depends_on_job_id":"%s","condition":"completed"}`,
		asString(t, upstream, "id"))
	w := doRequest(t, http.MethodPost, "/v1/jobs/"+asString(t, downstream, "id")+"/dependencies", depBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create dependency status = %d, body = %s", w.Code, w.Body.String())
	}
	dep := mustDecodeObject(t, w)

	w = doRequest(t, http.MethodGet, "/v1/jobs/"+asString(t, downstream, "id")+"/dependencies", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list dependencies status = %d, body = %s", w.Code, w.Body.String())
	}
	deps := mustDecodeList(t, w)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}
	if asString(t, deps[0], "id") != asString(t, dep, "id") {
		t.Fatalf("expected dependency id %s", asString(t, dep, "id"))
	}
}

func TestE2E_JobDependency_Delete(t *testing.T) {
	mustClean(t)

	projectID := "proj-dep-delete-" + newID()
	upstream := createJob(t, projectID, "Upstream", "upstream-"+newID())
	downstream := createJob(t, projectID, "Downstream", "downstream-"+newID())

	w := doRequest(t, http.MethodPost, "/v1/jobs/"+asString(t, downstream, "id")+"/dependencies",
		fmt.Sprintf(`{"depends_on_job_id":"%s"}`, asString(t, upstream, "id")))
	if w.Code != http.StatusCreated {
		t.Fatalf("create dependency status = %d, body = %s", w.Code, w.Body.String())
	}
	depID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodDelete, "/v1/jobs/"+asString(t, downstream, "id")+"/dependencies/"+depID, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete dependency status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/jobs/"+asString(t, downstream, "id")+"/dependencies", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list dependencies status = %d, body = %s", w.Code, w.Body.String())
	}
	deps := mustDecodeList(t, w)
	if len(deps) != 0 {
		t.Fatalf("expected 0 dependencies after deletion, got %d", len(deps))
	}
}

func TestE2E_JobDependency_SelfReferenceRejected(t *testing.T) {
	mustClean(t)

	projectID := "proj-dep-self-" + newID()
	job := createJob(t, projectID, "Self", "self-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, "/v1/jobs/"+jobID+"/dependencies",
		fmt.Sprintf(`{"depends_on_job_id":"%s"}`, jobID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("self-reference dependency status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestE2E_JobDependency_InvalidConditionRejected(t *testing.T) {
	mustClean(t)

	projectID := "proj-dep-cond-" + newID()
	upstream := createJob(t, projectID, "Upstream", "upstream-"+newID())
	downstream := createJob(t, projectID, "Downstream", "downstream-"+newID())

	w := doRequest(t, http.MethodPost, "/v1/jobs/"+asString(t, downstream, "id")+"/dependencies",
		fmt.Sprintf(`{"depends_on_job_id":"%s","condition":"never"}`, asString(t, upstream, "id")))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid dependency condition status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestE2E_Environment_CreateAndGet(t *testing.T) {
	mustClean(t)

	projectID := "proj-env-create-" + newID()
	w := doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Development","slug":"dev-%s","variables":{"REGION":"us-east-1"}}`,
		projectID, newID()))
	if w.Code != http.StatusCreated {
		t.Fatalf("create environment status = %d, body = %s", w.Code, w.Body.String())
	}
	created := mustDecodeObject(t, w)
	envID := asString(t, created, "id")

	w = doRequest(t, http.MethodGet, "/v1/environments/"+envID, "", projectID)
	if w.Code != http.StatusOK {
		t.Fatalf("get environment status = %d, body = %s", w.Code, w.Body.String())
	}
	env := mustDecodeObject(t, w)
	if asString(t, env, "id") != envID {
		t.Fatalf("expected environment id %s", envID)
	}
	w = doRequest(t, http.MethodGet, "/v1/environments/"+envID+"/variables", "", projectID)
	if w.Code != http.StatusOK {
		t.Fatalf("get resolved variables status = %d, body = %s", w.Code, w.Body.String())
	}
	resolved := asObject(t, mustDecodeObject(t, w), "variables")
	if asString(t, resolved, "REGION") != "us-east-1" {
		t.Fatalf("expected resolved REGION us-east-1, got %s", asString(t, resolved, "REGION"))
	}
}

func TestE2E_Environment_ListByProject(t *testing.T) {
	mustClean(t)

	projectID := "proj-env-list-" + newID()
	otherProjectID := "proj-env-list-other-" + newID()

	w := doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Dev","slug":"dev-%s"}`,
		projectID, newID()))
	if w.Code != http.StatusCreated {
		t.Fatalf("create env dev status = %d, body = %s", w.Code, w.Body.String())
	}
	w = doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Prod","slug":"prod-%s"}`,
		projectID, newID()))
	if w.Code != http.StatusCreated {
		t.Fatalf("create env prod status = %d, body = %s", w.Code, w.Body.String())
	}
	w = doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Other","slug":"other-%s"}`,
		otherProjectID, newID()))
	if w.Code != http.StatusCreated {
		t.Fatalf("create other env status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/environments/", "", projectID)
	if w.Code != http.StatusOK {
		t.Fatalf("list environments status = %d, body = %s", w.Code, w.Body.String())
	}
	envs := mustDecodeList(t, w)
	if len(envs) != 2 {
		t.Fatalf("expected 2 environments, got %d", len(envs))
	}
}

func TestE2E_Environment_Inheritance(t *testing.T) {
	mustClean(t)

	projectID := "proj-env-inherit-" + newID()
	w := doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Base","slug":"base-%s","variables":{"A":"1","B":"2"}}`,
		projectID, newID()))
	if w.Code != http.StatusCreated {
		t.Fatalf("create parent env status = %d, body = %s", w.Code, w.Body.String())
	}
	parentID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Child","slug":"child-%s","parent_id":"%s","variables":{"B":"child","C":"3"}}`,
		projectID, newID(), parentID))
	if w.Code != http.StatusCreated {
		t.Fatalf("create child env status = %d, body = %s", w.Code, w.Body.String())
	}
	childID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodGet, "/v1/environments/"+childID+"/variables", "", projectID)
	if w.Code != http.StatusOK {
		t.Fatalf("get resolved variables status = %d, body = %s", w.Code, w.Body.String())
	}
	variables := asObject(t, mustDecodeObject(t, w), "variables")
	if asString(t, variables, "A") != "1" {
		t.Fatalf("expected inherited A=1, got %s", asString(t, variables, "A"))
	}
	if asString(t, variables, "B") != "child" {
		t.Fatalf("expected child override B=child, got %s", asString(t, variables, "B"))
	}
	if asString(t, variables, "C") != "3" {
		t.Fatalf("expected child variable C=3, got %s", asString(t, variables, "C"))
	}
}

func TestE2E_Environment_Delete(t *testing.T) {
	mustClean(t)

	projectID := "proj-env-delete-" + newID()
	w := doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Temp","slug":"temp-%s"}`,
		projectID, newID()))
	if w.Code != http.StatusCreated {
		t.Fatalf("create environment status = %d, body = %s", w.Code, w.Body.String())
	}
	envID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodDelete, "/v1/environments/"+envID, "", projectID)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete environment status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/environments/"+envID, "", projectID)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get deleted environment status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestE2E_Environment_Update(t *testing.T) {
	mustClean(t)

	projectID := "proj-env-update-" + newID()
	w := doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Old","slug":"old-%s","variables":{"LOG_LEVEL":"info"}}`,
		projectID, newID()))
	if w.Code != http.StatusCreated {
		t.Fatalf("create environment status = %d, body = %s", w.Code, w.Body.String())
	}
	envID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodPatch, "/v1/environments/"+envID, `{"name":"New","slug":"new-slug","variables":{"LOG_LEVEL":"debug","REGION":"eu"}}`, projectID)
	if w.Code != http.StatusOK {
		t.Fatalf("update environment status = %d, body = %s", w.Code, w.Body.String())
	}
	updated := mustDecodeObject(t, w)
	if asString(t, updated, "name") != "New" {
		t.Fatalf("expected updated name, got %s", asString(t, updated, "name"))
	}
	w = doRequest(t, http.MethodGet, "/v1/environments/"+envID+"/variables", "", projectID)
	if w.Code != http.StatusOK {
		t.Fatalf("get updated variables status = %d, body = %s", w.Code, w.Body.String())
	}
	vars := asObject(t, mustDecodeObject(t, w), "variables")
	if asString(t, vars, "LOG_LEVEL") != "debug" {
		t.Fatalf("expected LOG_LEVEL=debug, got %s", asString(t, vars, "LOG_LEVEL"))
	}
}

func TestE2E_Environment_ResolvedVariablesEndpoint(t *testing.T) {
	mustClean(t)

	projectID := "proj-env-vars-" + newID()
	w := doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Base","slug":"base-%s","variables":{"TOKEN":"abc"}}`,
		projectID, newID()))
	if w.Code != http.StatusCreated {
		t.Fatalf("create environment status = %d, body = %s", w.Code, w.Body.String())
	}
	envID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodGet, "/v1/environments/"+envID+"/variables", "", projectID)
	if w.Code != http.StatusOK {
		t.Fatalf("get variables status = %d, body = %s", w.Code, w.Body.String())
	}
	vars := asObject(t, mustDecodeObject(t, w), "variables")
	if asString(t, vars, "TOKEN") != "abc" {
		t.Fatalf("expected TOKEN=abc, got %s", asString(t, vars, "TOKEN"))
	}
}

func TestE2E_JobHealth_GetStats(t *testing.T) {
	mustClean(t)

	projectID := "proj-health-" + newID()
	job := createJob(t, projectID, "Health Job", "health-"+newID())
	jobID := asString(t, job, "id")

	run1 := triggerJob(t, jobID, `{"payload":{"i":1}}`, "")
	run2 := triggerJob(t, jobID, `{"payload":{"i":2}}`, "")
	run3 := triggerJob(t, jobID, `{"payload":{"i":3}}`, "")

	ctx := context.Background()
	now := time.Now().UTC()

	run1ID := asString(t, run1, "id")
	if err := testStore.UpdateRunStatus(ctx, run1ID, domain.StatusQueued, domain.StatusDequeued, map[string]any{"started_at": now}); err != nil {
		t.Fatalf("run1 queued->dequeued: %v", err)
	}
	if err := testStore.UpdateRunStatus(ctx, run1ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{"started_at": now}); err != nil {
		t.Fatalf("run1 dequeued->executing: %v", err)
	}
	if err := testStore.UpdateRunStatus(ctx, run1ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{"finished_at": now.Add(2 * time.Second)}); err != nil {
		t.Fatalf("run1 executing->completed: %v", err)
	}

	run2ID := asString(t, run2, "id")
	if err := testStore.UpdateRunStatus(ctx, run2ID, domain.StatusQueued, domain.StatusDequeued, map[string]any{"started_at": now}); err != nil {
		t.Fatalf("run2 queued->dequeued: %v", err)
	}
	if err := testStore.UpdateRunStatus(ctx, run2ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{"started_at": now}); err != nil {
		t.Fatalf("run2 dequeued->executing: %v", err)
	}
	if err := testStore.UpdateRunStatus(ctx, run2ID, domain.StatusExecuting, domain.StatusFailed, map[string]any{"finished_at": now.Add(3 * time.Second), "error": "boom"}); err != nil {
		t.Fatalf("run2 executing->failed: %v", err)
	}

	run3ID := asString(t, run3, "id")
	if err := testStore.UpdateRunStatus(ctx, run3ID, domain.StatusQueued, domain.StatusCanceled, map[string]any{"finished_at": now.Add(1 * time.Second), "error": "canceled"}); err != nil {
		t.Fatalf("run3 queued->canceled: %v", err)
	}

	w := doRequest(t, http.MethodGet, "/v1/jobs/"+jobID+"/health?window=7d", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get health status = %d, body = %s", w.Code, w.Body.String())
	}
	health := mustDecodeObject(t, w)
	if asInt(t, health, "total_runs") != 3 {
		t.Fatalf("expected total_runs=3, got %d", asInt(t, health, "total_runs"))
	}
	if asInt(t, health, "completed_runs") != 1 {
		t.Fatalf("expected completed_runs=1, got %d", asInt(t, health, "completed_runs"))
	}
	if asInt(t, health, "failed_runs") != 1 {
		t.Fatalf("expected failed_runs=1, got %d", asInt(t, health, "failed_runs"))
	}
	if asInt(t, health, "canceled_runs") != 1 {
		t.Fatalf("expected canceled_runs=1, got %d", asInt(t, health, "canceled_runs"))
	}
}

func TestE2E_JobHealth_InvalidWindow(t *testing.T) {
	mustClean(t)

	projectID := "proj-health-window-" + newID()
	job := createJob(t, projectID, "Health Window", "health-window-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodGet, "/v1/jobs/"+jobID+"/health?window=2w", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("get health invalid window status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestE2E_BatchCreateJobs(t *testing.T) {
	mustClean(t)

	projectID := "proj-batch-create-" + newID()
	body := fmt.Sprintf(`{"jobs":[
		{"project_id":"%s","name":"Batch A","slug":"batch-a-%s","endpoint_url":"https://example.com/a"},
		{"project_id":"%s","name":"Batch B","slug":"batch-b-%s","endpoint_url":"https://example.com/b"},
		{"project_id":"%s","name":"Batch C","slug":"batch-c-%s","endpoint_url":"https://example.com/c"}
	]}`,
		projectID, newID(), projectID, newID(), projectID, newID())

	w := doRequest(t, http.MethodPost, "/v1/jobs/batch", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("batch create status = %d, body = %s", w.Code, w.Body.String())
	}
	resp := mustDecodeObject(t, w)
	createdRaw, ok := resp["created"].([]any)
	if !ok {
		t.Fatalf("created field type = %T, want []any", resp["created"])
	}
	if len(createdRaw) != 3 {
		t.Fatalf("expected 3 created jobs, got %d", len(createdRaw))
	}
}

func TestE2E_BatchEnableDisable(t *testing.T) {
	mustClean(t)

	projectID := "proj-batch-toggle-" + newID()
	job1 := createJob(t, projectID, "Toggle One", "toggle-one-"+newID())
	job2 := createJob(t, projectID, "Toggle Two", "toggle-two-"+newID())
	job1ID := asString(t, job1, "id")
	job2ID := asString(t, job2, "id")

	disableBody := fmt.Sprintf(`{"ids":["%s","%s"]}`, job1ID, job2ID)
	w := doRequest(t, http.MethodPost, "/v1/jobs/batch-disable", disableBody)
	if w.Code != http.StatusOK {
		t.Fatalf("batch disable status = %d, body = %s", w.Code, w.Body.String())
	}
	disableResp := mustDecodeObject(t, w)
	if asInt(t, disableResp, "updated") != 2 {
		t.Fatalf("expected updated=2 for disable, got %d", asInt(t, disableResp, "updated"))
	}

	w = doRequest(t, http.MethodGet, "/v1/jobs/"+job1ID+"/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get job1 after disable status = %d, body = %s", w.Code, w.Body.String())
	}
	if asBool(t, mustDecodeObject(t, w), "enabled") {
		t.Fatal("expected job1 enabled=false after batch-disable")
	}

	enableBody := fmt.Sprintf(`{"ids":["%s","%s"]}`, job1ID, job2ID)
	w = doRequest(t, http.MethodPost, "/v1/jobs/batch-enable", enableBody)
	if w.Code != http.StatusOK {
		t.Fatalf("batch enable status = %d, body = %s", w.Code, w.Body.String())
	}
	enableResp := mustDecodeObject(t, w)
	if asInt(t, enableResp, "updated") != 2 {
		t.Fatalf("expected updated=2 for enable, got %d", asInt(t, enableResp, "updated"))
	}

	w = doRequest(t, http.MethodGet, "/v1/jobs/"+job2ID+"/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get job2 after enable status = %d, body = %s", w.Code, w.Body.String())
	}
	if !asBool(t, mustDecodeObject(t, w), "enabled") {
		t.Fatal("expected job2 enabled=true after batch-enable")
	}
}

func TestE2E_BatchCreateJobs_PartialFailure(t *testing.T) {
	mustClean(t)

	projectID := "proj-batch-partial-" + newID()
	body := fmt.Sprintf(`{"jobs":[
		{"project_id":"%s","name":"Valid Job","slug":"valid-%s","endpoint_url":"https://example.com/valid"},
		{"project_id":"","name":"","slug":"","endpoint_url":""}
	]}`,
		projectID, newID())

	w := doRequest(t, http.MethodPost, "/v1/jobs/batch", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("batch partial status = %d, body = %s", w.Code, w.Body.String())
	}
	resp := mustDecodeObject(t, w)
	createdRaw, ok := resp["created"].([]any)
	if !ok {
		t.Fatalf("created field type = %T, want []any", resp["created"])
	}
	if len(createdRaw) != 1 {
		t.Fatalf("expected 1 created job, got %d", len(createdRaw))
	}
	errorsRaw, ok := resp["errors"].([]any)
	if !ok {
		t.Fatalf("errors field type = %T, want []any", resp["errors"])
	}
	if len(errorsRaw) != 1 {
		t.Fatalf("expected 1 batch error, got %d", len(errorsRaw))
	}
}

func TestE2E_BatchCreateJobs_AllInvalid(t *testing.T) {
	mustClean(t)

	body := `{"jobs":[{"project_id":"","name":"","slug":"","endpoint_url":""},{"project_id":"","name":"","slug":"","endpoint_url":""}]}`
	w := doRequest(t, http.MethodPost, "/v1/jobs/batch", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("batch all-invalid status = %d, body = %s", w.Code, w.Body.String())
	}
	resp := mustDecodeObject(t, w)
	errorsRaw, ok := resp["errors"].([]any)
	if !ok {
		t.Fatalf("errors field type = %T, want []any", resp["errors"])
	}
	if len(errorsRaw) != 2 {
		t.Fatalf("expected 2 batch errors, got %d", len(errorsRaw))
	}
}

func TestE2E_Secret_CreateAndList(t *testing.T) {
	mustClean(t)

	projectID := "proj-secret-create-" + newID()
	job := createJob(t, projectID, "Secret Job", "secret-job-"+newID())
	jobID := asString(t, job, "id")

	secretBody := fmt.Sprintf(`{"project_id":"%s","job_id":"%s","environment":"dev","secret_key":"API_KEY","value":"super-secret"}`,
		projectID, jobID)
	w := doRequest(t, http.MethodPost, "/v1/secrets/", secretBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create secret status = %d, body = %s", w.Code, w.Body.String())
	}
	secret := mustDecodeObject(t, w)
	if asString(t, secret, "id") == "" {
		t.Fatal("expected secret id")
	}

	w = doRequest(t, http.MethodGet, "/v1/secrets/?job_id="+jobID+"&environment=dev", "", projectID)
	if w.Code != http.StatusOK {
		t.Fatalf("list secrets status = %d, body = %s", w.Code, w.Body.String())
	}
	secrets := mustDecodeList(t, w)
	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(secrets))
	}
	if asString(t, secrets[0], "secret_key") != "API_KEY" {
		t.Fatalf("expected secret_key API_KEY, got %s", asString(t, secrets[0], "secret_key"))
	}
	if _, exists := secrets[0]["encrypted_value"]; exists {
		t.Fatal("secret list should not expose encrypted_value")
	}
}

func TestE2E_Secret_Delete(t *testing.T) {
	mustClean(t)

	projectID := "proj-secret-delete-" + newID()
	job := createJob(t, projectID, "Secret Delete Job", "secret-delete-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, "/v1/secrets/", fmt.Sprintf(`{"project_id":"%s","job_id":"%s","environment":"dev","secret_key":"TOKEN","value":"abc123"}`,
		projectID, jobID))
	if w.Code != http.StatusCreated {
		t.Fatalf("create secret status = %d, body = %s", w.Code, w.Body.String())
	}
	secretID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodDelete, "/v1/secrets/"+secretID, "", projectID)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete secret status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/secrets/?job_id="+jobID+"&environment=dev", "", projectID)
	if w.Code != http.StatusOK {
		t.Fatalf("list secrets after delete status = %d, body = %s", w.Code, w.Body.String())
	}
	secrets := mustDecodeList(t, w)
	if len(secrets) != 0 {
		t.Fatalf("expected 0 secrets after delete, got %d", len(secrets))
	}
}

func TestE2E_Secret_DefaultEnvironment(t *testing.T) {
	mustClean(t)

	projectID := "proj-secret-default-" + newID()
	job := createJob(t, projectID, "Secret Default Job", "secret-default-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, "/v1/secrets/", fmt.Sprintf(`{"project_id":"%s","job_id":"%s","secret_key":"DB_URL","value":"postgres://x"}`,
		projectID, jobID))
	if w.Code != http.StatusCreated {
		t.Fatalf("create secret default env status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/secrets/?job_id="+jobID+"&environment=production", "", projectID)
	if w.Code != http.StatusOK {
		t.Fatalf("list production secrets status = %d, body = %s", w.Code, w.Body.String())
	}
	secrets := mustDecodeList(t, w)
	if len(secrets) != 1 {
		t.Fatalf("expected 1 production secret, got %d", len(secrets))
	}
}

func TestE2E_DryRun_ValidPayload(t *testing.T) {
	mustClean(t)

	projectID := "proj-dry-valid-" + newID()
	job := createJob(t, projectID, "Dry Run", "dry-run-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{"dry_run":true,"payload":{"name":"alice","age":30}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("dry-run valid status = %d, body = %s", w.Code, w.Body.String())
	}
	resp := mustDecodeObject(t, w)
	jobObj := asObject(t, resp, "job")
	if asString(t, jobObj, "id") != jobID {
		t.Fatalf("expected dry-run job id %s", jobID)
	}
	if asString(t, resp, "payload_hash") == "" {
		t.Fatal("expected dry-run payload_hash")
	}
}

func TestE2E_DryRun_InvalidPayload(t *testing.T) {
	mustClean(t)

	projectID := "proj-dry-invalid-" + newID()
	slug := "dry-invalid-" + newID()
	createBody := fmt.Sprintf(`{"project_id":"%s","name":"Dry Invalid","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60,"payload_schema":{"type":"object","required":["name"]}}`,
		projectID, slug, slug)
	w := doRequest(t, http.MethodPost, "/v1/jobs/", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create job for invalid dry-run status = %d, body = %s", w.Code, w.Body.String())
	}
	jobID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{"dry_run":true,"payload":{"age":22}}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("dry-run invalid payload status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "payload validation failed") {
		t.Fatalf("expected payload validation failure message, got %s", w.Body.String())
	}
}

func TestE2E_CloneJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-clone-" + newID()
	source := createJob(t, projectID, "Source Job", "source-"+newID())
	sourceID := asString(t, source, "id")

	cloneSlug := "clone-" + newID()
	body := fmt.Sprintf(`{"name":"Cloned Job","slug":"%s"}`, cloneSlug)
	w := doRequest(t, http.MethodPost, "/v1/jobs/"+sourceID+"/clone", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("clone job status = %d, body = %s", w.Code, w.Body.String())
	}
	clone := mustDecodeObject(t, w)
	if asString(t, clone, "id") == sourceID {
		t.Fatal("expected clone id to differ from source")
	}
	if asString(t, clone, "slug") != cloneSlug {
		t.Fatalf("expected clone slug %s, got %s", cloneSlug, asString(t, clone, "slug"))
	}
	if asString(t, clone, "project_id") != projectID {
		t.Fatalf("expected clone project_id %s", projectID)
	}
}

func TestE2E_CloneJob_NotFound(t *testing.T) {
	mustClean(t)

	w := doRequest(t, http.MethodPost, "/v1/jobs/non-existent-job/clone", `{"name":"Clone","slug":"clone-`+newID()+`"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("clone missing job status = %d, body = %s", w.Code, w.Body.String())
	}
}
