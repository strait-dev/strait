import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect, selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

// The key template interpolates the trigger payload, so two triggers with
// different `tenant` values resolve to two distinct locks and run in parallel,
// each holding its own key. A single stamp keeps the two keys unique per run.
const STAMP = Date.now();
const TENANT_A = `alpha-${STAMP}`;
const TENANT_B = `beta-${STAMP}`;
// Both holder runs sleep this long so their locks stay held while the dashboard
// loads and both tests assert against them.
const HOLDER_DELAY_MS = 60_000;

let api: ApiHelper;
let data: TestDataFactory;
let jobId: string;
let jobName: string;
let holders: { lock_key: string; holder_run_id: string }[];

test.describe("Singletons dashboard with multiple keys", () => {
  test.describe.configure({ timeout: 120_000 });

  test.beforeAll(async () => {
    api = new ApiHelper();
    data = new TestDataFactory(api);

    const job = await data.job("singleton-multikey", {
      endpoint_url: api.fakeEndpoint(`/timeout?delay_ms=${HOLDER_DELAY_MS}`),
      timeout_secs: 90,
      max_attempts: 1,
      // biome-ignore lint/suspicious/noTemplateCurlyInString: backend key template, interpolated server-side from the trigger payload
      singleton_key_expr: { template: "${tenant}" },
      singleton_on_conflict: "queue",
    });
    jobId = job.id;
    jobName = job.name;

    // Both holder runs stay in-flight; a job cannot be deleted while it has
    // active runs, so drain them before the data.job() delete (cleanup is LIFO).
    data.cleanup.add(() => api.cancelJobRuns(jobId));

    // Two triggers with distinct tenants acquire two separate locks.
    await Promise.all([
      api.triggerJob(jobId, { tenant: TENANT_A }),
      api.triggerJob(jobId, { tenant: TENANT_B }),
    ]);

    // Gate on the backend holding both keys before asserting the UI.
    holders = await api.waitForJobSingletonHolders(jobId, 2, 30_000);
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("shows a holder row for each distinct key", async ({ page }) => {
    await gotoAndExpect(
      page,
      `/app/jobs/${jobId}`,
      page.getByRole("heading", { name: jobName })
    );

    await selectTab(page, "Singletons");

    const table = page.getByRole("table", { name: "Singleton holders" });
    await expect(table).toBeVisible();
    await expect(table.getByText(TENANT_A)).toBeVisible();
    await expect(table.getByText(TENANT_B)).toBeVisible();

    // Each key links to the run that holds it. Match on the full run href
    // rather than the truncated link text: two runs triggered in the same
    // millisecond can share their first 8 id characters.
    for (const holder of holders) {
      await expect(
        table.locator(`a[href="/app/runs/${holder.holder_run_id}"]`)
      ).toBeVisible();
    }
  });

  test("the holder run detail surfaces its singleton key", async ({ page }) => {
    const holder = holders.find((h) => h.lock_key === TENANT_A);
    if (!holder) {
      throw new Error(`No holder found for key ${TENANT_A}`);
    }

    await gotoAndExpect(
      page,
      `/app/runs/${holder.holder_run_id}`,
      page.getByRole("heading", { name: holder.holder_run_id })
    );

    // The persisted singleton key is rendered in the run timeline footer. The
    // tenant value only appears here as this run's singleton key, so its
    // presence is an unambiguous assertion that the key surfaced.
    await expect(page.getByText(TENANT_A).first()).toBeVisible();
  });
});
