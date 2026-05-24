import { randomUUID } from "node:crypto";
import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

const terminalWorkflowStatuses = [
  "completed",
  "failed",
  "timed_out",
  "canceled",
];

async function waitForStepRefs(
  api: ApiHelper,
  workflowRunId: string,
  expectedRefs: string[]
) {
  await expect(async () => {
    const steps = await api.listWorkflowStepRuns(workflowRunId, { limit: 20 });
    expect(steps.data.map((step) => step.step_ref).sort()).toEqual(
      [...expectedRefs].sort()
    );
  }).toPass({ timeout: 30_000 });
}

async function stepStatusMap(api: ApiHelper, workflowRunId: string) {
  const steps = await api.listWorkflowStepRuns(workflowRunId, { limit: 20 });
  return new Map(steps.data.map((step) => [step.step_ref, step.status]));
}

test.describe("Workflow execution edge cases", () => {
  test.describe.configure({ timeout: 240_000 });

  let api: ApiHelper;
  let data: TestDataFactory;

  test.beforeEach(() => {
    api = new ApiHelper();
    data = new TestDataFactory(api);
  });

  test.afterEach(async () => {
    await data?.cleanup.run();
  });

  test("bootstraps a fan-out/fan-in workflow with every step observable", async () => {
    const root = await data.job("wf-fan-root");
    const branchA = await data.job("wf-fan-a");
    const branchB = await data.job("wf-fan-b");
    const join = await data.job("wf-fan-join");
    const workflow = await api.createWorkflow({
      name: data.name("wf-fan-in"),
      steps: [
        { job_id: root.id, step_ref: "root" },
        { job_id: branchA.id, step_ref: "branch-a", depends_on: ["root"] },
        { job_id: branchB.id, step_ref: "branch-b", depends_on: ["root"] },
        {
          job_id: join.id,
          step_ref: "join",
          depends_on: ["branch-a", "branch-b"],
        },
      ],
    });
    data.cleanup.add(() => api.deleteWorkflow(workflow.id));

    const run = await api.triggerWorkflow(workflow.id, {
      scenario: "fan-out-fan-in",
    });
    await waitForStepRefs(api, run.id, [
      "root",
      "branch-a",
      "branch-b",
      "join",
    ]);
    await api.cancelWorkflowRun(run.id);
  });

  test("bootstraps failure policies and independent branches", async () => {
    const toleratedFailure = await data.job("wf-continue-fail", {
      endpoint_url: api.fakeEndpoint("/status/500"),
    });
    const independentSuccess = await data.job("wf-continue-success");
    const workflow = await api.createWorkflow({
      name: data.name("wf-continue-policy"),
      steps: [
        {
          job_id: toleratedFailure.id,
          step_ref: "tolerated-failure",
          on_failure: "continue",
        },
        { job_id: independentSuccess.id, step_ref: "independent-success" },
      ],
    });
    data.cleanup.add(() => api.deleteWorkflow(workflow.id));

    const run = await api.triggerWorkflow(workflow.id, {
      scenario: "continue-on-failure",
    });
    await waitForStepRefs(api, run.id, [
      "tolerated-failure",
      "independent-success",
    ]);
    await api.cancelWorkflowRun(run.id);
  });

  test("completes wait-for-event workflows and records the step transition", async () => {
    const eventKey = data.name("wf-event-complete");
    const workflow = await api.createWorkflow({
      name: data.name("wf-event-complete"),
      steps: [
        {
          step_ref: "await-signal",
          step_type: "wait_for_event",
          event_key: eventKey,
          event_timeout_secs: 300,
        },
      ],
    });
    data.cleanup.add(() => api.deleteWorkflow(workflow.id));

    const run = await api.triggerWorkflow(workflow.id, {
      scenario: "event-complete",
    });
    data.cleanup.add(() =>
      api.sendEvent(eventKey, { cleanup: true }).catch(() => undefined)
    );
    await api.waitForEventTrigger(
      (event) =>
        event.event_key === eventKey &&
        event.workflow_run_id === run.id &&
        event.status === "waiting",
      60_000
    );

    await api.sendEvent(eventKey, { approved: true });
    const completed = await api.waitForWorkflowRunStatus(
      run.id,
      ["completed"],
      120_000
    );
    expect(completed.status).toBe("completed");

    const statuses = await stepStatusMap(api, run.id);
    expect(statuses.get("await-signal")).toBe("completed");
  });

  test("bootstraps conditional guards as observable step runs", async () => {
    const root = await data.job("wf-condition-root");
    const guarded = await data.job("wf-condition-guarded");
    const workflow = await api.createWorkflow({
      name: data.name("wf-condition-skip"),
      steps: [
        { job_id: root.id, step_ref: "root" },
        {
          job_id: guarded.id,
          step_ref: "guarded",
          depends_on: ["root"],
          condition: {
            type: "step_status",
            step_ref: "root",
            status: "failed",
          },
        },
      ],
    });
    data.cleanup.add(() => api.deleteWorkflow(workflow.id));

    const run = await api.triggerWorkflow(workflow.id, {
      scenario: "condition-skip",
    });
    await waitForStepRefs(api, run.id, ["root", "guarded"]);
    await api.cancelWorkflowRun(run.id);
  });

  test("rejects cyclic workflow definitions before they can execute", async () => {
    const first = await data.job("wf-cycle-a");
    const second = await data.job("wf-cycle-b");

    const response = await api.requestRaw("POST", "/v1/workflows", {
      project_id: api.getProjectId(),
      name: data.name("wf-cycle"),
      slug: `e2e-wf-cycle-${randomUUID().slice(0, 8)}`,
      steps: [
        { job_id: first.id, step_ref: "a", depends_on: ["b"] },
        { job_id: second.id, step_ref: "b", depends_on: ["a"] },
      ],
    });

    expect(response.status).toBe(400);
    expect(response.text.toLowerCase()).toMatch(/cycle|dag|depend/);
  });

  test("cancels long-running workflow runs and leaves terminal state observable", async () => {
    const longJob = await data.job("wf-cancel-long", {
      endpoint_url: api.fakeEndpoint("/timeout?delay_ms=10000"),
      timeout_secs: 15,
    });
    const workflow = await api.createWorkflow({
      name: data.name("wf-cancel"),
      steps: [{ job_id: longJob.id, step_ref: "long-step" }],
    });
    data.cleanup.add(() => api.deleteWorkflow(workflow.id));

    const run = await api.triggerWorkflow(workflow.id, { scenario: "cancel" });
    await api.waitForWorkflowRunStatus(run.id, ["running"], 30_000);
    await api.cancelWorkflowRun(run.id);
    const canceled = await api.waitForWorkflowRunStatus(
      run.id,
      terminalWorkflowStatuses,
      60_000
    );

    expect(canceled.status).toBe("canceled");
  });
});
