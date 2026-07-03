import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

let api: ApiHelper;
let data: TestDataFactory;
let runId: string;

test.describe("Dashboard metrics and activity", () => {
  test.describe.configure({ timeout: 90_000 });
  test.setTimeout(90_000);

  test.beforeAll(async () => {
    api = new ApiHelper();
    data = new TestDataFactory(api);
    const job = await data.job("dashboard-metrics");
    const run = await api.triggerJob(job.id, { source: "dashboard-metrics" });
    runId = run.id;
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("exposes queue stats and performance analytics through the Go API", async () => {
    await api.waitForRunStatus(runId, ["completed", "succeeded"], 60_000);

    const stats = await api.getStats();
    expect(stats.queued).toBeGreaterThanOrEqual(0);
    expect(stats.executing).toBeGreaterThanOrEqual(0);
    expect(stats.delayed).toBeGreaterThanOrEqual(0);

    const analytics = await api.getAnalytics(24);
    expect(analytics.health_summary.total_jobs).toBeGreaterThanOrEqual(1);
    expect(
      analytics.throughput.completed + analytics.throughput.failed
    ).toBeGreaterThanOrEqual(1);
  });

  test("renders dashboard cards, charts, and recent backend runs", async ({
    page,
  }) => {
    const consoleMessages: string[] = [];
    page.on("console", (message) => {
      if (message.type() === "warning" || message.type() === "error") {
        consoleMessages.push(message.text());
      }
    });

    await page.goto("/app/dashboard", { waitUntil: "domcontentloaded" });

    await expect(page.getByText("Total runs (24h)")).toBeVisible();
    await expect(page.getByText("Success rate")).toBeVisible();
    await expect(page.getByText("Failed runs", { exact: true })).toBeVisible();
    await expect(page.getByText("Queued")).toBeVisible();
    await expect(page.getByText("Recent runs", { exact: true })).toBeVisible();
    await expect(page.getByText(runId.slice(0, 8))).toBeVisible({
      timeout: 15_000,
    });
    expect(
      consoleMessages.filter((message) => /width\(-1\)/i.test(message))
    ).toEqual([]);
  });
});
