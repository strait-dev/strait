import type { Page } from "@playwright/test";
import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect, selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

// A run only offers continue-as-new while it is running or paused, so these
// scenarios hold the run open with a slow fake endpoint long enough to drive
// the UI and build a continuation chain through the API. The window has to
// outlast multi-step UI interactions (validation, version-strategy selection),
// so the endpoint sleeps well past the time the dashboard needs.
const SLOW_ENDPOINT = "/timeout?delay_ms=20000";
// The job timeout must outlast the endpoint sleep so the run stays running
// (rather than timing out) for the whole interaction window.
const SLOW_JOB_TIMEOUT_SECS = 25;

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

  /** Open a workflow's Recent Runs tab, resilient to dev-loader drops. */
  async function gotoRecentRuns(page: Page, workflowId: string) {
    await gotoAndExpect(
      page,
      `/app/workflows/${workflowId}`,
      page.getByRole("tab", { name: "Recent Runs" })
    );
    await selectTab(page, "Recent Runs");
  }

  /** Locate the runs-table row for a given run by its short id prefix. */
  function runRow(page: Page, runId: string) {
    return page.getByRole("row").filter({ hasText: runId.slice(0, 8) });
  }

  /** Seed a workflow whose single step stays running for the SLOW_ENDPOINT. */
  async function slowWorkflow(prefix: string) {
    const job = await data.job(prefix, {
      endpoint_url: api.fakeEndpoint(SLOW_ENDPOINT),
      timeout_secs: SLOW_JOB_TIMEOUT_SECS,
    });
    return data.workflow(prefix, [job.id]);
  }

  test("continues a running workflow run as new from the actions menu", async ({
    page,
  }) => {
    const workflow = await slowWorkflow("wf-continue-action");

    const run = await api.triggerWorkflow(workflow.id, { cursor: 0 });
    await api.waitForWorkflowRunStatus(run.id, ["running"], 30_000);

    await gotoRecentRuns(page, workflow.id);

    await runRow(page, run.id)
      .getByRole("button", { name: "Run actions" })
      .click();
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

  test("rejects invalid carry-over input before continuing with the latest version strategy", async ({
    page,
  }) => {
    const workflow = await slowWorkflow("wf-continue-validate");

    const run = await api.triggerWorkflow(workflow.id, { cursor: 0 });
    await api.waitForWorkflowRunStatus(run.id, ["running"], 30_000);

    await gotoRecentRuns(page, workflow.id);

    await runRow(page, run.id)
      .getByRole("button", { name: "Run actions" })
      .click();
    await page.getByRole("menuitem", { name: "Continue as new" }).click();

    const dialog = page.getByRole("dialog");
    const input = dialog.getByLabel("Carry-over input (JSON, optional)");
    const submit = dialog.getByRole("button", { name: "Continue as new" });

    // Malformed JSON is rejected inline and never reaches the backend.
    await input.fill("{cursor: }");
    await submit.click();
    await expect(dialog.getByText("Input must be valid JSON.")).toBeVisible();

    const untouched = await api.getWorkflowRun(run.id);
    expect(untouched.status).toBe("running");
    expect(untouched.continued_to_workflow_run_id).toBeFalsy();

    // Switching to the latest-version strategy and supplying valid JSON
    // submits successfully through the same dialog.
    await dialog.getByRole("combobox").click();
    await page.getByRole("option", { name: /Latest/ }).click();
    await input.fill('{"cursor": 9}');
    await submit.click();

    await expect(page.getByText(/Started successor run/i)).toBeVisible();

    const predecessor = await api.waitForWorkflowRunStatus(
      run.id,
      ["continued"],
      30_000
    );
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
    const workflow = await slowWorkflow("wf-continue-chain");

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

    await gotoRecentRuns(page, workflow.id);

    const rootRow = runRow(page, root.id);
    await expect(
      rootRow.getByLabel("Part of a continuation chain")
    ).toBeVisible();

    // The root has been continued, so it is terminal: the menu offers chain
    // navigation but no longer offers continue-as-new.
    await rootRow.getByRole("button", { name: "Run actions" }).click();
    await expect(
      page.getByRole("menuitem", { name: "View chain" })
    ).toBeVisible();
    await expect(
      page.getByRole("menuitem", { name: "Continue as new" })
    ).toHaveCount(0);
    await page.getByRole("menuitem", { name: "View chain" }).click();

    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("Continuation chain")).toBeVisible();

    const items = dialog.getByRole("listitem");
    await expect(items).toHaveCount(3);

    // Rendered oldest-first: root (#0) at the top, latest (#2) at the bottom.
    await expect(items.nth(0)).toContainText("#0");
    await expect(items.nth(0)).toContainText(root.id.slice(0, 8));
    await expect(items.nth(2)).toContainText("#2");
    await expect(items.nth(2)).toContainText(latest.id.slice(0, 8));

    // The run the dialog was opened from is the only one flagged current.
    await expect(items.nth(0).getByText("Current")).toBeVisible();
    await expect(items.nth(2).getByText("Current")).toHaveCount(0);
  });
});
