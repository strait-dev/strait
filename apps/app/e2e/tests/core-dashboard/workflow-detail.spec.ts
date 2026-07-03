import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect, selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

test.describe("Workflow detail dashboard", () => {
  test.describe.configure({ timeout: 150_000 });
  test.setTimeout(150_000);

  let api: ApiHelper;
  let data: TestDataFactory;

  test.beforeEach(() => {
    api = new ApiHelper();
    data = new TestDataFactory(api);
  });

  test.afterEach(async () => {
    await data?.cleanup.run();
  });

  test("renders workflow runs, DAG steps, settings, and pause state", async ({
    page,
  }) => {
    const job = await data.job("workflow-detail-step");
    const workflow = await api.createWorkflow({
      name: data.name("workflow-detail"),
      description: "E2E workflow detail coverage",
      steps: [
        {
          job_id: job.id,
          step_ref: "fetch-record",
          depends_on: [],
        },
      ],
    });
    data.cleanup.add(() => api.deleteWorkflow(workflow.id));

    const workflowRun = await api.triggerWorkflow(workflow.id, {
      source: "workflow-detail-e2e",
    });
    const observedRun = await api.waitForWorkflowRunStatus(
      workflowRun.id,
      ["running", "completed", "failed"],
      30_000
    );

    await gotoAndExpect(
      page,
      `/app/workflows/${workflow.id}`,
      page.getByRole("heading", { name: workflow.name })
    );

    await expect(page.getByText("Total runs")).toBeVisible();
    await expect(
      page.getByText(observedRun.id.slice(0, 8), { exact: true })
    ).toBeVisible();
    await expect(
      page.getByText(/running|completed|failed/i).first()
    ).toBeVisible();

    await selectTab(page, "Recent runs");
    await expect(
      page.getByRole("table", { name: "Workflow runs" })
    ).toBeVisible();
    await expect(
      page.getByRole("row", {
        name: new RegExp(
          `${observedRun.id.slice(0, 8)}\\s+(running|completed|failed)`,
          "i"
        ),
      })
    ).toBeVisible();

    await selectTab(page, "DAG");
    await expect(page.getByText("fetch-record")).toBeVisible();
    await expect(page.getByText("Job").first()).toBeVisible();

    await selectTab(page, "Settings");
    await expect(page.getByText("Version policy")).toBeVisible();
    await expect(page.getByText("Manual")).toBeVisible();

    await page.getByRole("button", { name: "Pause" }).click();
    await expect(page.getByRole("button", { name: "Resume" })).toBeVisible();
    await expect
      .poll(async () => (await api.getWorkflow(workflow.id)).enabled)
      .toBe(false);

    await page.getByRole("button", { name: "Resume" }).click();
    await expect(page.getByRole("button", { name: "Pause" })).toBeVisible();
    await expect
      .poll(async () => (await api.getWorkflow(workflow.id)).enabled)
      .toBe(true);
  });

  test("surfaces failed workflow runs from a real failed step", async ({
    page,
  }) => {
    const failingJob = await data.job("workflow-detail-failure", {
      endpoint_url: api.fakeEndpoint("/fail"),
      max_attempts: 1,
      timeout_secs: 5,
    });
    const workflow = await api.createWorkflow({
      name: data.name("workflow-detail-failure"),
      steps: [
        {
          job_id: failingJob.id,
          step_ref: "failing-step",
          depends_on: [],
        },
      ],
    });
    data.cleanup.add(() => api.deleteWorkflow(workflow.id));

    const workflowRun = await api.triggerWorkflow(workflow.id, {
      source: "workflow-detail-failure-e2e",
    });
    const failedRun = await api.waitForWorkflowRunStatus(
      workflowRun.id,
      ["failed"],
      90_000
    );

    await gotoAndExpect(
      page,
      `/app/workflows/${workflow.id}`,
      page.getByRole("heading", { name: workflow.name })
    );

    await expect(
      page.getByText(failedRun.id.slice(0, 8), { exact: true })
    ).toBeVisible();
    await selectTab(page, "Recent runs");
    await expect(
      page.getByRole("row", {
        name: new RegExp(`${failedRun.id.slice(0, 8)}\\s+failed`, "i"),
      })
    ).toBeVisible();
  });
});
