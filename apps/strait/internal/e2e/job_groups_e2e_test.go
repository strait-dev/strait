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

	"github.com/stretchr/testify/require"
)

func TestE2E_JobGroup_CreateAndGet(t *testing.T) {
	mustClean(t)

	projectID := "proj-group-create-" + newID()
	body := fmt.Sprintf(`{"project_id":"%s","name":"Core Jobs","slug":"core-jobs-%s","description":"Core pipelines"}`,
		projectID, newID())

	w := doRequest(t, http.MethodPost, "/v1/job-groups/", body)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	created := mustDecodeObject(t, w)
	groupID := asString(t, created, "id")

	w = doRequest(t, http.MethodGet, "/v1/job-groups/"+groupID, "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	fetched := mustDecodeObject(t, w)
	require.Equal(t, groupID,

		asString(t, fetched,
			"id",
		))
	require.Equal(t, projectID,

		asString(t, fetched,

			"project_id",
		))

}

func TestE2E_JobGroup_ListByProject(t *testing.T) {
	mustClean(t)

	projectID := "proj-group-list-" + newID()
	otherProjectID := "proj-group-list-other-" + newID()

	w := doRequest(t, http.MethodPost, "/v1/job-groups/", fmt.Sprintf(`{"project_id":"%s","name":"Alpha","slug":"alpha-%s"}`,
		projectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	w = doRequest(t, http.MethodPost, "/v1/job-groups/", fmt.Sprintf(`{"project_id":"%s","name":"Beta","slug":"beta-%s"}`,
		projectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	w = doRequest(t, http.MethodPost, "/v1/job-groups/", fmt.Sprintf(`{"project_id":"%s","name":"Other","slug":"other-%s"}`,
		otherProjectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	w = doRequest(t, http.MethodGet, "/v1/job-groups/", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	groups := mustDecodeList(t, w)
	require.Len(t, groups,

		2)

}

func TestE2E_JobGroup_Update(t *testing.T) {
	mustClean(t)

	projectID := "proj-group-update-" + newID()
	w := doRequest(t, http.MethodPost, "/v1/job-groups/", fmt.Sprintf(`{"project_id":"%s","name":"Old Name","slug":"old-%s"}`,
		projectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	groupID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodPatch, "/v1/job-groups/"+groupID, `{"name":"New Name","description":"Updated description"}`)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	updated := mustDecodeObject(t, w)
	require.Equal(t, "New Name",

		asString(t, updated,

			"name"))
	require.Equal(t, "Updated description",

		asString(
			t, updated,
			"description",
		))

}

func TestE2E_JobGroup_DeleteAndListJobs(t *testing.T) {
	mustClean(t)

	projectID := "proj-group-delete-" + newID()
	createGroup := doRequest(t, http.MethodPost, "/v1/job-groups/", fmt.Sprintf(`{"project_id":"%s","name":"To Delete","slug":"del-%s"}`,
		projectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		createGroup.
			Code)

	groupID := asString(t, mustDecodeObject(t, createGroup), "id")

	jobBody := fmt.Sprintf(`{"project_id":"%s","group_id":"%s","name":"Grouped Job","slug":"grouped-%s","endpoint_url":"https://example.com/grouped","max_attempts":3,"timeout_secs":60}`,
		projectID, groupID, newID())
	w := doRequest(t, http.MethodPost, "/v1/jobs/", jobBody)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	w = doRequest(t, http.MethodDelete, "/v1/job-groups/"+groupID, "")
	require.Equal(t, http.
		StatusNoContent,
		w.Code,
	)

	w = doRequest(t, http.MethodGet, "/v1/job-groups/"+groupID+"/jobs", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	jobs := mustDecodeList(t, w)
	require.Len(t, jobs,
		0,
	)

}

func TestE2E_JobGroup_ListJobsByGroup(t *testing.T) {
	mustClean(t)

	projectID := "proj-group-jobs-" + newID()
	createGroup := doRequest(t, http.MethodPost, "/v1/job-groups/", fmt.Sprintf(`{"project_id":"%s","name":"Ops","slug":"ops-%s"}`,
		projectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		createGroup.
			Code)

	groupID := asString(t, mustDecodeObject(t, createGroup), "id")

	jobBody1 := fmt.Sprintf(`{"project_id":"%s","group_id":"%s","name":"Grouped One","slug":"grouped-one-%s","endpoint_url":"https://example.com/grouped-one","max_attempts":3,"timeout_secs":60}`,
		projectID, groupID, newID())
	w := doRequest(t, http.MethodPost, "/v1/jobs/", jobBody1)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	jobBody2 := fmt.Sprintf(`{"project_id":"%s","group_id":"%s","name":"Grouped Two","slug":"grouped-two-%s","endpoint_url":"https://example.com/grouped-two","max_attempts":3,"timeout_secs":60}`,
		projectID, groupID, newID())
	w = doRequest(t, http.MethodPost, "/v1/jobs/", jobBody2)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	w = doRequest(t, http.MethodGet, "/v1/job-groups/"+groupID+"/jobs", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	jobs := mustDecodeList(t, w)
	require.Len(t, jobs,
		2,
	)

}

func TestE2E_JobDependency_CreateAndList(t *testing.T) {
	mustClean(t)

	projectID := "proj-dep-create-" + newID()
	upstream := createJob(t, projectID, "Upstream", "upstream-"+newID())
	downstream := createJob(t, projectID, "Downstream", "downstream-"+newID())

	depBody := fmt.Sprintf(`{"depends_on_job_id":"%s","condition":"completed"}`,
		asString(t, upstream, "id"))
	w := doRequest(t, http.MethodPost, "/v1/jobs/"+asString(t, downstream, "id")+"/dependencies", depBody)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	dep := mustDecodeObject(t, w)

	w = doRequest(t, http.MethodGet, "/v1/jobs/"+asString(t, downstream, "id")+"/dependencies", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	deps := mustDecodeList(t, w)
	require.Len(t, deps,
		1,
	)
	require.Equal(t, asString(t, dep,
		"id"), asString(t, deps[0],
		"id"))

}

func TestE2E_JobDependency_Delete(t *testing.T) {
	mustClean(t)

	projectID := "proj-dep-delete-" + newID()
	upstream := createJob(t, projectID, "Upstream", "upstream-"+newID())
	downstream := createJob(t, projectID, "Downstream", "downstream-"+newID())

	w := doRequest(t, http.MethodPost, "/v1/jobs/"+asString(t, downstream, "id")+"/dependencies",
		fmt.Sprintf(`{"depends_on_job_id":"%s"}`, asString(t, upstream, "id")))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	depID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodDelete, "/v1/jobs/"+asString(t, downstream, "id")+"/dependencies/"+depID, "")
	require.Equal(t, http.
		StatusNoContent,
		w.Code,
	)

	w = doRequest(t, http.MethodGet, "/v1/jobs/"+asString(t, downstream, "id")+"/dependencies", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	deps := mustDecodeList(t, w)
	require.Len(t, deps,
		0,
	)

}

func TestE2E_JobDependency_SelfReferenceRejected(t *testing.T) {
	mustClean(t)

	projectID := "proj-dep-self-" + newID()
	job := createJob(t, projectID, "Self", "self-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, "/v1/jobs/"+jobID+"/dependencies",
		fmt.Sprintf(`{"depends_on_job_id":"%s"}`, jobID))
	require.Equal(t, http.
		StatusBadRequest,
		w.
			Code)

}

func TestE2E_JobDependency_InvalidConditionRejected(t *testing.T) {
	mustClean(t)

	projectID := "proj-dep-cond-" + newID()
	upstream := createJob(t, projectID, "Upstream", "upstream-"+newID())
	downstream := createJob(t, projectID, "Downstream", "downstream-"+newID())

	w := doRequest(t, http.MethodPost, "/v1/jobs/"+asString(t, downstream, "id")+"/dependencies",
		fmt.Sprintf(`{"depends_on_job_id":"%s","condition":"never"}`, asString(t, upstream, "id")))
	require.Equal(t, http.
		StatusBadRequest,
		w.
			Code)

}

func TestE2E_Environment_CreateAndGet(t *testing.T) {
	mustClean(t)

	projectID := "proj-env-create-" + newID()
	w := doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Development","slug":"dev-%s","variables":{"REGION":"us-east-1"}}`,
		projectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	created := mustDecodeObject(t, w)
	envID := asString(t, created, "id")

	w = doRequest(t, http.MethodGet, "/v1/environments/"+envID, "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	env := mustDecodeObject(t, w)
	require.Equal(t, envID,

		asString(t, env, "id"))

	w = doRequest(t, http.MethodGet, "/v1/environments/"+envID+"/variables", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	resolved := asObject(t, mustDecodeObject(t, w), "variables")
	require.Equal(t, "us-east-1",

		asString(t,
			resolved,
			"REGION"),
	)

}

func TestE2E_Environment_ListByProject(t *testing.T) {
	mustClean(t)

	projectID := "proj-env-list-" + newID()
	otherProjectID := "proj-env-list-other-" + newID()

	w := doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Dev","slug":"dev-%s"}`,
		projectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	w = doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Prod","slug":"prod-%s"}`,
		projectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	w = doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Other","slug":"other-%s"}`,
		otherProjectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	w = doRequest(t, http.MethodGet, "/v1/environments/", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	envs := mustDecodeList(t, w)
	require.Len(t, envs,
		2,
	)

}

func TestE2E_Environment_Inheritance(t *testing.T) {
	mustClean(t)

	projectID := "proj-env-inherit-" + newID()
	w := doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Base","slug":"base-%s","variables":{"A":"1","B":"2"}}`,
		projectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	parentID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Child","slug":"child-%s","parent_id":"%s","variables":{"B":"child","C":"3"}}`,
		projectID, newID(), parentID))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	childID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodGet, "/v1/environments/"+childID+"/variables", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	variables := asObject(t, mustDecodeObject(t, w), "variables")
	require.Equal(t, "1",

		asString(
			t, variables,
			"A"),
	)
	require.Equal(t, "child",

		asString(t, variables,

			"B"))
	require.Equal(t, "3",

		asString(
			t, variables,
			"C"),
	)

}

func TestE2E_Environment_Delete(t *testing.T) {
	mustClean(t)

	projectID := "proj-env-delete-" + newID()
	w := doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Temp","slug":"temp-%s"}`,
		projectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	envID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodDelete, "/v1/environments/"+envID, "")
	require.Equal(t, http.
		StatusNoContent,
		w.Code,
	)

	w = doRequest(t, http.MethodGet, "/v1/environments/"+envID, "")
	require.Equal(t, http.
		StatusNotFound,
		w.Code,
	)

}

func TestE2E_Environment_Update(t *testing.T) {
	mustClean(t)

	projectID := "proj-env-update-" + newID()
	w := doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Old","slug":"old-%s","variables":{"LOG_LEVEL":"info"}}`,
		projectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	envID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodPatch, "/v1/environments/"+envID, `{"name":"New","slug":"new-slug","variables":{"LOG_LEVEL":"debug","REGION":"eu"}}`)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	updated := mustDecodeObject(t, w)
	require.Equal(t, "New",

		asString(t, updated,
			"name",
		))

	w = doRequest(t, http.MethodGet, "/v1/environments/"+envID+"/variables", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	vars := asObject(t, mustDecodeObject(t, w), "variables")
	require.Equal(t, "debug",

		asString(t, vars,
			"LOG_LEVEL",
		))

}

func TestE2E_Environment_ResolvedVariablesEndpoint(t *testing.T) {
	mustClean(t)

	projectID := "proj-env-vars-" + newID()
	w := doRequest(t, http.MethodPost, "/v1/environments/", fmt.Sprintf(`{"project_id":"%s","name":"Base","slug":"base-%s","variables":{"TOKEN":"abc"}}`,
		projectID, newID()))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	envID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodGet, "/v1/environments/"+envID+"/variables", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	vars := asObject(t, mustDecodeObject(t, w), "variables")
	require.Equal(t, "abc",

		asString(t, vars,
			"TOKEN",
		))

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
	require.NoError(t, testStore.
		UpdateRunStatus(ctx,
			run1ID, domain.
				StatusQueued,
			domain.
				StatusDequeued,

			map[string]any{"started_at": now}))
	require.NoError(t, testStore.
		UpdateRunStatus(ctx,
			run1ID, domain.
				StatusDequeued,
			domain.
				StatusExecuting,

			map[string]any{
				"started_at": now}))
	require.NoError(t, testStore.
		UpdateRunStatus(ctx,
			run1ID, domain.
				StatusExecuting,
			domain.
				StatusCompleted,

			map[string]any{
				"finished_at": now.Add(2 * time.Second)}))

	run2ID := asString(t, run2, "id")
	require.NoError(t, testStore.
		UpdateRunStatus(ctx,
			run2ID, domain.
				StatusQueued,
			domain.
				StatusDequeued,

			map[string]any{"started_at": now}))
	require.NoError(t, testStore.
		UpdateRunStatus(ctx,
			run2ID, domain.
				StatusDequeued,
			domain.
				StatusExecuting,

			map[string]any{
				"started_at": now}))
	require.NoError(t, testStore.
		UpdateRunStatus(ctx,
			run2ID, domain.
				StatusExecuting,
			domain.
				StatusFailed,

			map[string]any{
				"finished_at": now.Add(3 * time.Second), "error": "boom",
			}))

	run3ID := asString(t, run3, "id")
	require.NoError(t, testStore.
		UpdateRunStatus(ctx,
			run3ID, domain.
				StatusQueued,
			domain.
				StatusCanceled,

			map[string]any{"finished_at": now.Add(1 * time.Second), "error": "canceled"}))

	w := doRequest(t, http.MethodGet, "/v1/jobs/"+jobID+"/health?window=7d", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	health := mustDecodeObject(t, w)
	require.EqualValues(t, 3, asInt(t, health,
		"total_runs",
	))
	require.EqualValues(t, 1, asInt(t, health,
		"completed_runs",
	))
	require.EqualValues(t, 1, asInt(t, health,
		"failed_runs",
	))
	require.EqualValues(t, 1, asInt(t, health,
		"canceled_runs",
	))

}

func TestE2E_JobHealth_InvalidWindow(t *testing.T) {
	mustClean(t)

	projectID := "proj-health-window-" + newID()
	job := createJob(t, projectID, "Health Window", "health-window-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodGet, "/v1/jobs/"+jobID+"/health?window=2w", "")
	require.Equal(t, http.
		StatusBadRequest,
		w.
			Code)

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
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	resp := mustDecodeObject(t, w)
	createdRaw, ok := resp["created"].([]any)
	require.True(t, ok)
	require.Len(t, createdRaw,

		3)

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
	require.Equal(t, http.
		StatusOK,
		w.Code)

	disableResp := mustDecodeObject(t, w)
	require.EqualValues(t, 2, asInt(t, disableResp,

		"updated",
	))

	w = doRequest(t, http.MethodGet, "/v1/jobs/"+job1ID+"/", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.False(t, asBool(t, mustDecodeObject(t, w),
		"enabled"),
	)

	enableBody := fmt.Sprintf(`{"ids":["%s","%s"]}`, job1ID, job2ID)
	w = doRequest(t, http.MethodPost, "/v1/jobs/batch-enable", enableBody)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	enableResp := mustDecodeObject(t, w)
	require.EqualValues(t, 2, asInt(t, enableResp,
		"updated",
	))

	w = doRequest(t, http.MethodGet, "/v1/jobs/"+job2ID+"/", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.True(t, asBool(t, mustDecodeObject(t, w),
		"enabled"))

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
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	resp := mustDecodeObject(t, w)
	createdRaw, ok := resp["created"].([]any)
	require.True(t, ok)
	require.Len(t, createdRaw,

		1)

	errorsRaw, ok := resp["errors"].([]any)
	require.True(t, ok)
	require.Len(t, errorsRaw,

		1)

}

func TestE2E_BatchCreateJobs_AllInvalid(t *testing.T) {
	mustClean(t)

	body := `{"jobs":[{"project_id":"","name":"","slug":"","endpoint_url":""},{"project_id":"","name":"","slug":"","endpoint_url":""}]}`
	w := doRequest(t, http.MethodPost, "/v1/jobs/batch", body)
	require.Equal(t, http.
		StatusBadRequest,
		w.
			Code)

	resp := mustDecodeObject(t, w)
	errorsRaw, ok := resp["errors"].([]any)
	require.True(t, ok)
	require.Len(t, errorsRaw,

		2)

}

func TestE2E_Secret_CreateAndList(t *testing.T) {
	mustClean(t)

	projectID := "proj-secret-create-" + newID()
	job := createJob(t, projectID, "Secret Job", "secret-job-"+newID())
	jobID := asString(t, job, "id")

	secretBody := fmt.Sprintf(`{"project_id":"%s","job_id":"%s","environment":"dev","secret_key":"API_KEY","value":"super-secret"}`,
		projectID, jobID)
	w := doRequest(t, http.MethodPost, "/v1/secrets/", secretBody)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	secret := mustDecodeObject(t, w)
	require.NotEqual(t, "",

		asString(t, secret,
			"id"),
	)

	w = doRequest(t, http.MethodGet, "/v1/secrets/?job_id="+jobID+"&environment=dev", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	secrets := mustDecodeList(t, w)
	require.Len(t, secrets,

		1)
	require.Equal(t, "API_KEY",

		asString(t, secrets[0], "secret_key"))

	if _, exists := secrets[0]["encrypted_value"]; exists {
		require.Fail(t,

			"secret list should not expose encrypted_value")
	}
}

func TestE2E_Secret_Delete(t *testing.T) {
	mustClean(t)

	projectID := "proj-secret-delete-" + newID()
	job := createJob(t, projectID, "Secret Delete Job", "secret-delete-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, "/v1/secrets/", fmt.Sprintf(`{"project_id":"%s","job_id":"%s","environment":"dev","secret_key":"TOKEN","value":"abc123"}`,
		projectID, jobID))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	secretID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodDelete, "/v1/secrets/"+secretID, "")
	require.Equal(t, http.
		StatusNoContent,
		w.Code,
	)

	w = doRequest(t, http.MethodGet, "/v1/secrets/?job_id="+jobID+"&environment=dev", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	secrets := mustDecodeList(t, w)
	require.Len(t, secrets,

		0)

}

func TestE2E_Secret_DefaultEnvironment(t *testing.T) {
	mustClean(t)

	projectID := "proj-secret-default-" + newID()
	job := createJob(t, projectID, "Secret Default Job", "secret-default-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, "/v1/secrets/", fmt.Sprintf(`{"project_id":"%s","job_id":"%s","secret_key":"DB_URL","value":"postgres://x"}`,
		projectID, jobID))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	w = doRequest(t, http.MethodGet, "/v1/secrets/?job_id="+jobID+"&environment=production", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	secrets := mustDecodeList(t, w)
	require.Len(t, secrets,

		1)

}

func TestE2E_DryRun_ValidPayload(t *testing.T) {
	mustClean(t)

	projectID := "proj-dry-valid-" + newID()
	job := createJob(t, projectID, "Dry Run", "dry-run-"+newID())
	jobID := asString(t, job, "id")

	w := doRequest(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{"dry_run":true,"payload":{"name":"alice","age":30}}`)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	resp := mustDecodeObject(t, w)
	jobObj := asObject(t, resp, "job")
	require.Equal(t, jobID,

		asString(t, jobObj,
			"id"),
	)
	require.NotEqual(t, "",

		asString(t, resp,
			"payload_hash",
		))

}

func TestE2E_DryRun_InvalidPayload(t *testing.T) {
	mustClean(t)

	projectID := "proj-dry-invalid-" + newID()
	slug := "dry-invalid-" + newID()
	createBody := fmt.Sprintf(`{"project_id":"%s","name":"Dry Invalid","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60,"payload_schema":{"type":"object","required":["name"]}}`,
		projectID, slug, slug)
	w := doRequest(t, http.MethodPost, "/v1/jobs/", createBody)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	jobID := asString(t, mustDecodeObject(t, w), "id")

	w = doRequest(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{"dry_run":true,"payload":{"age":22}}`)
	require.Equal(t, http.
		StatusBadRequest,
		w.
			Code)
	require.True(t, strings.Contains(w.Body.String(),
		"payload validation failed",
	))

}

func TestE2E_CloneJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-clone-" + newID()
	source := createJob(t, projectID, "Source Job", "source-"+newID())
	sourceID := asString(t, source, "id")

	cloneSlug := "clone-" + newID()
	body := fmt.Sprintf(`{"name":"Cloned Job","slug":"%s"}`, cloneSlug)
	w := doRequest(t, http.MethodPost, "/v1/jobs/"+sourceID+"/clone", body)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	clone := mustDecodeObject(t, w)
	require.NotEqual(t, sourceID,

		asString(t,
			clone,
			"id"))
	require.Equal(t, cloneSlug,

		asString(t, clone,
			"slug",
		))
	require.Equal(t, projectID,

		asString(t, clone,
			"project_id",
		))

}

func TestE2E_CloneJob_NotFound(t *testing.T) {
	mustClean(t)

	w := doRequest(t, http.MethodPost, "/v1/jobs/non-existent-job/clone", `{"name":"Clone","slug":"clone-`+newID()+`"}`)
	require.Equal(t, http.
		StatusNotFound,
		w.Code,
	)

}
