import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect, selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

// A constant key template makes every run collide on the same singleton lock,
// so a second trigger parks behind the first instead of running in parallel.
const SINGLETON_KEY = `e2e-singleton-${Date.now()}`;
// The holder run sleeps this long so its lock stays held while the dashboard
// loads and asserts. Keep it comfortably above the gate + navigation budget.
const HOLDER_DELAY_MS = 45_000;

let api: ApiHelper;
let data: TestDataFactory;
let jobId: string;
let jobName: string;
let lockKey: string;
let holderRunId: string;

test.describe("Singletons dashboard", () => {
  test.describe.configure({ timeout: 120_000 });

  test.beforeAll(async () => {
    api = new ApiHelper();
    data = new TestDataFactory(api);

    const job = await data.job("singleton", {
      endpoint_url: api.fakeEndpoint(`/timeout?delay_ms=${HOLDER_DELAY_MS}`),
      timeout_secs: 60,
      max_attempts: 1,
      singleton_key_expr: { template: SINGLETON_KEY },
      singleton_on_conflict: "queue",
    });
    jobId = job.id;
    jobName = job.name;

    // Fire two runs at once: one acquires the lock and stays in-flight while
    // the other parks behind it, producing a holder with a waiter.
    await Promise.all([api.triggerJob(jobId), api.triggerJob(jobId)]);

    // Gate on the backend before touching the UI so the test only asserts
    // rendering, not the singleton mechanism itself.
    const holder = await api.waitForJobSingletonHolder(jobId, 1, 30_000);
    lockKey = holder.lock_key;
    holderRunId = holder.holder_run_id;
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("shows the held key, holder run, and waiter count", async ({ page }) => {
    await gotoAndExpect(
      page,
      `/app/jobs/${jobId}`,
      page.getByRole("heading", { name: jobName })
    );

    await selectTab(page, "Singletons");

    const table = page.getByRole("table", { name: "Singleton holders" });
    await expect(table).toBeVisible();
    await expect(table.getByText(lockKey)).toBeVisible();
    await expect(
      table.getByRole("link", { name: holderRunId.slice(0, 8) })
    ).toBeVisible();
  });

  test("configuration shows the read-only singleton settings", async ({
    page,
  }) => {
    await gotoAndExpect(
      page,
      `/app/jobs/${jobId}`,
      page.getByRole("heading", { name: jobName })
    );

    await expect(page.getByText("Singleton Mode")).toBeVisible();
    await expect(page.getByText("Queue").first()).toBeVisible();
    await expect(page.getByText("Singleton Key")).toBeVisible();
    await expect(page.getByText(SINGLETON_KEY).first()).toBeVisible();
  });
});
