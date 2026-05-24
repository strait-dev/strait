import { ApiHelper, expect, test } from "../../fixtures";
import { selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

let api: ApiHelper;
let data: TestDataFactory;
let scheduleId: string;
let scheduleName: string;

test.describe("Schedule detail page", () => {
  test.describe.configure({ timeout: 120_000 });
  test.setTimeout(120_000);

  test.beforeAll(async () => {
    api = new ApiHelper();
    data = new TestDataFactory(api);

    const schedule = await api.createSchedule({
      name: data.name("schedule-detail"),
      endpoint_url: api.fakeEndpoint("/success"),
      cron: "*/10 * * * *",
      timeout_secs: 10,
    });
    data.cleanup.add(() => api.deleteSchedule(schedule.id));
    scheduleId = schedule.id;
    scheduleName = schedule.name;
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("renders schedule configuration and run history", async ({ page }) => {
    await page.goto(`/app/schedules/${scheduleId}`, {
      waitUntil: "domcontentloaded",
    });

    await expect(
      page.getByRole("heading", { name: scheduleName })
    ).toBeVisible();
    await expect(page.getByText("*/10 * * * *")).toBeVisible();
    await expect(page.getByRole("button", { name: "Trigger" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Pause" })).toBeVisible();

    await selectTab(page, "Settings");
    await expect(page.getByText("Configuration")).toBeVisible();
    await expect(page.getByText(api.fakeEndpoint("/success"))).toBeVisible();
    await expect(page.getByText("10s")).toBeVisible();
  });

  test("reflects triggered, paused, and resumed schedule state", async ({
    page,
  }) => {
    const before = await api.listRuns({ job_id: scheduleId, limit: 10 });

    await page.goto(`/app/schedules/${scheduleId}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(page.getByRole("button", { name: "Trigger" })).toBeVisible();
    const run = await api.triggerJob(scheduleId, { source: "schedule-e2e" });

    await expect(async () => {
      const after = await api.listRuns({ job_id: scheduleId, limit: 10 });
      expect(after.data.length).toBeGreaterThan(before.data.length);
    }).toPass({ timeout: 30_000 });
    await api.cancelRun(run.id).catch(() => undefined);

    await api.updateJob(scheduleId, { enabled: false });
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect
      .poll(async () => (await api.getJob(scheduleId)).enabled)
      .toBe(false);
    await expect(page.getByRole("button", { name: "Resume" })).toBeVisible();

    await api.updateJob(scheduleId, { enabled: true });
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect
      .poll(async () => (await api.getJob(scheduleId)).enabled)
      .toBe(true);
    await expect(page.getByRole("button", { name: "Pause" })).toBeVisible();
  });

  test("shows not-found state for invalid schedule IDs", async ({ page }) => {
    await page.goto("/app/schedules/not-a-real-schedule", {
      waitUntil: "domcontentloaded",
    });

    await expect(page.locator("main")).toBeVisible();
    await expect(
      page.getByText(/not found|couldn't find|error/i)
    ).toBeVisible();
  });
});
