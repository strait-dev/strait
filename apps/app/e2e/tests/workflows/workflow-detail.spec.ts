import { ApiHelper, expect, test } from "../../fixtures";

const runId = Date.now();
const workflowName = `e2e-core-workflow-detail-${runId}`;
const rootJobName = `e2e-wf-detail-root-${runId}`;
const childJobName = `e2e-wf-detail-child-${runId}`;

let api: ApiHelper;
let workflowId: string;
let rootJobId: string;
let childJobId: string;

test.describe("Workflow Detail Page", () => {
  test.beforeAll(async () => {
    api = new ApiHelper();
    const rootJob = await api.createJob({
      name: rootJobName,
      endpoint_url: api.fakeEndpoint("/success"),
    });
    const childJob = await api.createJob({
      name: childJobName,
      endpoint_url: api.fakeEndpoint("/success"),
    });
    rootJobId = rootJob.id;
    childJobId = childJob.id;

    const workflow = await api.createWorkflow({
      name: workflowName,
      description: "Workflow detail seeded by Playwright",
      steps: [
        { job_id: rootJobId, step_ref: "root" },
        { job_id: childJobId, step_ref: "child", depends_on: ["root"] },
      ],
    });
    workflowId = workflow.id;
  });

  test.afterAll(async () => {
    await Promise.allSettled([
      workflowId ? api.deleteWorkflow(workflowId) : Promise.resolve(),
      rootJobId ? api.deleteJob(rootJobId) : Promise.resolve(),
      childJobId ? api.deleteJob(childJobId) : Promise.resolve(),
    ]);
  });

  test("overview shows actions, metrics, and recent activity", async ({
    page,
  }) => {
    await page.goto(`/app/workflows/${workflowId}`, {
      waitUntil: "domcontentloaded",
    });

    await expect(
      page.getByRole("heading", { name: workflowName })
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Trigger" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Pause" })).toBeVisible();
    await expect(page.getByText("Success Rate")).toBeVisible();
    await expect(page.getByText("Total Runs")).toBeVisible();
    await expect(
      page.getByText("Recent Activity", { exact: true })
    ).toBeVisible();
  });
});
