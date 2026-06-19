import { ApiHelper, expect, test } from "../../fixtures";

const runId = Date.now();
const workflowName = `e2e-core-workflow-${runId}`;
const rootJobName = `e2e-wf-root-${runId}`;
const childJobName = `e2e-wf-child-${runId}`;

let api: ApiHelper;
let workflowId: string;
let rootJobId: string;
let childJobId: string;

test.describe("Workflows", () => {
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
      description: "Workflow seeded by Playwright",
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

  test.beforeEach(async ({ page }) => {
    await page.goto("/app/workflows", { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("region", { name: "Workflows" })).toBeVisible();
  });

  test("renders controls and the seeded workflow", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/workflows/);
    await expect(page.getByPlaceholder("Search workflows...")).toBeVisible();
    await expect(page.getByRole("button", { name: "Status" })).toBeVisible();
    await expect(page.getByText(workflowName)).toBeVisible();
  });

  test("accepts search input without losing the seeded workflow", async ({
    page,
  }) => {
    await page.getByPlaceholder("Search workflows...").fill(workflowName);

    await expect(page.getByPlaceholder("Search workflows...")).toHaveValue(
      workflowName
    );
    await expect(page.getByText(workflowName)).toBeVisible();
  });
});
