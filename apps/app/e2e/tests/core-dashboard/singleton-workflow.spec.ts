import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect, selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

// Unique key templates so each getByText(key) assertion is unambiguous and the
// workflows never collide on a shared singleton lock.
const STAMP = Date.now();
const DROP_KEY = `wf-drop-${STAMP}`;
const DEPTH_KEY = `wf-depth-${STAMP}`;
const EMPTY_KEY = `wf-empty-${STAMP}`;
const MAX_QUEUE_DEPTH = 5;

type Workflow = { id: string; name: string };

/** The read-only singleton config rows live in the workflow Settings tab. */
function configRow(page: import("@playwright/test").Page, label: string) {
  return page
    .locator("div.items-center.justify-between")
    .filter({ hasText: label })
    .last();
}

test.describe("Singleton workflow configuration display", () => {
  test.describe.configure({ timeout: 120_000 });

  let api: ApiHelper;
  let data: TestDataFactory;
  let dropWf: Workflow;
  let depthWf: Workflow;
  let emptyWf: Workflow;
  let plainWf: Workflow;

  test.beforeAll(async () => {
    api = new ApiHelper();
    data = new TestDataFactory(api);

    // Every workflow needs at least one step; a single plain job backs them all.
    // None of these workflows are triggered, so the job never runs and no lock
    // is ever held — the config rows and empty holders table render on their own.
    const stepJob = await data.job("singleton-wf-step");

    [dropWf, depthWf, emptyWf, plainWf] = await Promise.all([
      data.workflow("singleton-wf-drop", [stepJob.id], {
        singleton_key_expr: { template: DROP_KEY },
        singleton_on_conflict: "drop",
      }),
      data.workflow("singleton-wf-depth", [stepJob.id], {
        singleton_key_expr: { template: DEPTH_KEY },
        singleton_on_conflict: "queue",
        singleton_max_queue_depth: MAX_QUEUE_DEPTH,
      }),
      data.workflow("singleton-wf-empty", [stepJob.id], {
        singleton_key_expr: { template: EMPTY_KEY },
        singleton_on_conflict: "queue",
      }),
      data.workflow("plain-wf", [stepJob.id]),
    ]);
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("drop policy shows mode and key but no queue-depth row", async ({
    page,
  }) => {
    await gotoAndExpect(
      page,
      `/app/workflows/${dropWf.id}`,
      page.getByRole("heading", { name: dropWf.name })
    );

    await selectTab(page, "Settings");

    await expect(page.getByText("Singleton Mode")).toBeVisible();
    await expect(configRow(page, "Singleton Mode")).toContainText("Drop");
    await expect(page.getByText("Singleton Key")).toBeVisible();
    await expect(page.getByText(DROP_KEY).first()).toBeVisible();
    // Max Queue Depth is only meaningful for the queue policy.
    await expect(page.getByText("Max Queue Depth")).toHaveCount(0);
  });

  test("queue policy shows the configured max queue depth", async ({
    page,
  }) => {
    await gotoAndExpect(
      page,
      `/app/workflows/${depthWf.id}`,
      page.getByRole("heading", { name: depthWf.name })
    );

    await selectTab(page, "Settings");

    await expect(configRow(page, "Singleton Mode")).toContainText("Queue");
    await expect(page.getByText(DEPTH_KEY).first()).toBeVisible();
    await expect(page.getByText("Max Queue Depth")).toBeVisible();
    await expect(configRow(page, "Max Queue Depth")).toContainText(
      String(MAX_QUEUE_DEPTH)
    );
  });

  test("a singleton workflow with no runs shows the empty holders table", async ({
    page,
  }) => {
    await gotoAndExpect(
      page,
      `/app/workflows/${emptyWf.id}`,
      page.getByRole("heading", { name: emptyWf.name })
    );

    await selectTab(page, "Singletons");

    await expect(
      page.getByRole("table", { name: "Singleton holders" })
    ).toBeVisible();
    await expect(page.getByText("No keys currently held")).toBeVisible();
  });

  test("a non-singleton workflow hides the singleton tab and config", async ({
    page,
  }) => {
    await gotoAndExpect(
      page,
      `/app/workflows/${plainWf.id}`,
      page.getByRole("heading", { name: plainWf.name })
    );

    await expect(page.getByRole("tab", { name: "Singletons" })).toHaveCount(0);

    await selectTab(page, "Settings");
    await expect(page.getByText("Singleton Mode")).toHaveCount(0);
  });
});

test.describe("Singleton workflow holders", () => {
  test.describe.configure({ timeout: 120_000 });

  // The key template interpolates the workflow trigger payload, so two triggers
  // with different `tenant` values resolve to two distinct locks and run in
  // parallel, each holding its own key (workflow concurrency is unlimited).
  const HOLDER_STAMP = Date.now();
  const TENANT_A = `wf-alpha-${HOLDER_STAMP}`;
  const TENANT_B = `wf-beta-${HOLDER_STAMP}`;
  // The step job sleeps this long so each workflow run stays in-flight and keeps
  // its lock held while the dashboard loads and asserts.
  const HOLDER_DELAY_MS = 60_000;

  let api: ApiHelper;
  let data: TestDataFactory;
  let workflowId: string;
  let workflowName: string;
  let holders: { lock_key: string; holder_run_id: string }[];

  test.beforeAll(async () => {
    api = new ApiHelper();
    data = new TestDataFactory(api);

    // A long-running step keeps the workflow run (and thus its singleton lock)
    // in-flight for the duration of the test.
    const stepJob = await data.job("singleton-wf-holder-step", {
      endpoint_url: api.fakeEndpoint(`/timeout?delay_ms=${HOLDER_DELAY_MS}`),
      timeout_secs: 90,
      max_attempts: 1,
    });

    const workflow = await data.workflow("singleton-wf-holder", [stepJob.id], {
      // biome-ignore lint/suspicious/noTemplateCurlyInString: backend key template, interpolated server-side from the trigger payload
      singleton_key_expr: { template: "${tenant}" },
      singleton_on_conflict: "queue",
    });
    workflowId = workflow.id;
    workflowName = workflow.name;

    // Both workflow runs stay in-flight. A workflow cannot be deleted while it
    // has active runs, and the step job cannot be deleted while its runs are
    // live. Cancel the workflow runs first (cascading to their steps), then
    // drain any leftover job runs, before the data factory's deletes. Cleanup is
    // LIFO, so the workflow-run cancel is registered last to run first.
    data.cleanup.add(() => api.cancelJobRuns(stepJob.id));
    data.cleanup.add(() => api.cancelWorkflowRuns(workflowId));

    // Two triggers with distinct tenants acquire two separate locks.
    await Promise.all([
      api.triggerWorkflow(workflowId, { tenant: TENANT_A }),
      api.triggerWorkflow(workflowId, { tenant: TENANT_B }),
    ]);

    // Gate on the backend holding both keys before asserting the UI.
    holders = await api.waitForWorkflowSingletonHolders(workflowId, 2, 30_000);
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("shows a holder row for each distinct key", async ({ page }) => {
    await gotoAndExpect(
      page,
      `/app/workflows/${workflowId}`,
      page.getByRole("heading", { name: workflowName })
    );

    await selectTab(page, "Singletons");

    const table = page.getByRole("table", { name: "Singleton holders" });
    await expect(table).toBeVisible();
    await expect(table.getByText(TENANT_A)).toBeVisible();
    await expect(table.getByText(TENANT_B)).toBeVisible();

    // Each key links to the run that holds it. Match on the full run href rather
    // than the truncated link text: two runs triggered in the same millisecond
    // can share their first 8 id characters.
    for (const holder of holders) {
      await expect(
        table.locator(`a[href="/app/runs/${holder.holder_run_id}"]`)
      ).toBeVisible();
    }
  });
});
