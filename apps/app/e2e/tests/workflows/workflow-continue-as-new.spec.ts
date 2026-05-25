import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

// A run only offers continue-as-new while it is running or paused, so these
// scenarios hold the run open with a slow fake endpoint long enough to drive
// the UI and build a continuation chain through the API.
const SLOW_ENDPOINT = "/timeout?delay_ms=10000";

test.describe("Workflow continue-as-new", () => {
  test.describe.configure({ timeout: 180_000 });

  let api: ApiHelper;
  let data: TestDataFactory;

  test.beforeEach(() => {
    api = new ApiHelper();
    data = new TestDataFactory(api);
  });

  test.afterEach(async () => {
    await data.cleanup.run();
  });

  test("continues a running workflow run as new from the actions menu", async ({
    page,
  }) => {
    const job = await data.job("wf-continue-action", {
      endpoint_url: api.fakeEndpoint(SLOW_ENDPOINT),
      timeout_secs: 15,
    });
    const workflow = await data.workflow("wf-continue-action", [job.id]);

    const run = await api.triggerWorkflow(workflow.id, { cursor: 0 });
    await api.waitForWorkflowRunStatus(run.id, ["running"], 30_000);

    await page.goto(`/app/workflows/${workflow.id}`, {
      waitUntil: "domcontentloaded",
    });
    await page.getByRole("tab", { name: "Recent Runs" }).click();

    const runRow = page
      .getByRole("row")
      .filter({ hasText: run.id.slice(0, 8) });
    await runRow.getByRole("button", { name: "Run actions" }).click();
    await page.getByRole("menuitem", { name: "Continue as new" }).click();

    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByText("Complete run", { exact: false })
    ).toBeVisible();
    await dialog
      .getByLabel("Carry-over input (JSON, optional)")
      .fill('{"cursor": 1}');
    await dialog.getByRole("button", { name: "Continue as new" }).click();

    await expect(page.getByText(/Started successor run/i)).toBeVisible();

    // The predecessor is handed off to the successor and becomes terminal.
    const predecessor = await api.waitForWorkflowRunStatus(
      run.id,
      ["continued"],
      30_000
    );
    expect(predecessor.continued_to_workflow_run_id).toBeTruthy();

    const successorId = predecessor.continued_to_workflow_run_id;
    if (!successorId) {
      throw new Error("predecessor is missing a successor link");
    }
    const successor = await api.getWorkflowRun(successorId);
    expect(successor.continued_from_workflow_run_id).toBe(run.id);
    expect(successor.lineage_depth).toBe(1);
  });

  test("shows the lineage indicator and chain dialog for a continued run", async ({
    page,
  }) => {
    const job = await data.job("wf-continue-chain", {
      endpoint_url: api.fakeEndpoint(SLOW_ENDPOINT),
      timeout_secs: 15,
    });
    const workflow = await data.workflow("wf-continue-chain", [job.id]);

    // Build a three-run chain through the API: root -> mid -> latest.
    const root = await api.triggerWorkflow(workflow.id, { cursor: 0 });
    await api.waitForWorkflowRunStatus(root.id, ["running"], 30_000);
    const mid = await api.continueWorkflowRunAsNew(root.id, {
      input: { cursor: 1 },
    });
    await api.waitForWorkflowRunStatus(mid.id, ["running"], 30_000);
    const latest = await api.continueWorkflowRunAsNew(mid.id, {
      input: { cursor: 2 },
    });

    const chain = await api.getWorkflowRunChain(latest.id);
    expect(chain.data.map((entry) => entry.lineage_depth)).toEqual([0, 1, 2]);
    expect(chain.data[0].id).toBe(root.id);

    await page.goto(`/app/workflows/${workflow.id}`, {
      waitUntil: "domcontentloaded",
    });
    await page.getByRole("tab", { name: "Recent Runs" }).click();

    const rootRow = page
      .getByRole("row")
      .filter({ hasText: root.id.slice(0, 8) });
    await expect(
      rootRow.getByLabel("Part of a continuation chain")
    ).toBeVisible();

    await rootRow.getByRole("button", { name: "Run actions" }).click();
    await page.getByRole("menuitem", { name: "View chain" }).click();

    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("Continuation chain")).toBeVisible();
    await expect(dialog.getByRole("listitem")).toHaveCount(3);
    await expect(dialog.getByText(root.id.slice(0, 8))).toBeVisible();
  });
});
