import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

let api: ApiHelper;
let data: TestDataFactory;
let scheduleName: string;
let workflowName: string;

test.describe("Operational dashboard surfaces", () => {
  test.describe.configure({ timeout: 90_000 });
  test.setTimeout(90_000);

  test.beforeAll(async () => {
    api = new ApiHelper();
    data = new TestDataFactory(api);

    const schedule = await api.createSchedule({
      name: data.name("schedule"),
      endpoint_url: api.fakeEndpoint("/success"),
      cron: "*/15 * * * *",
      timeout_secs: 10,
    });
    data.cleanup.add(() => api.deleteSchedule(schedule.id));
    scheduleName = schedule.name;

    const rootJob = await data.job("workflow-root");
    const childJob = await data.job("workflow-child");
    const workflow = await data.workflow("workflow", [rootJob.id, childJob.id]);
    workflowName = workflow.name;

    await data.webhook("ops", ["run.completed", "run.failed"]);
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("renders schedules backed by cron jobs", async ({ page }) => {
    await page.goto("/app/schedules", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("region", { name: "Schedules" })).toBeVisible();
    await page.getByPlaceholder("Search schedules...").fill(scheduleName);
    await expect(page.getByText(scheduleName)).toBeVisible();
    await expect(page.getByText("*/15 * * * *")).toBeVisible();
  });

  test("renders workflows created through the Go API", async ({ page }) => {
    await page.goto("/app/workflows", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("region", { name: "Workflows" })).toBeVisible();
    await page.getByPlaceholder("Search workflows...").fill(workflowName);
    await expect(page.getByText(workflowName)).toBeVisible();
  });

  test("renders webhook subscriptions, events, and dead letter queue surfaces", async ({
    page,
  }) => {
    await page.goto("/app/webhooks", { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("table", { name: "Webhooks" })).toBeVisible();
    await expect(
      page
        .getByRole("link", { name: "Create webhook" })
        .or(page.getByRole("button", { name: "Create webhook" }))
    ).toBeVisible();

    await page.goto("/app/events", { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: "All" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Waiting" })).toBeVisible();

    await page.goto("/app/dlq", { waitUntil: "domcontentloaded" });
    await expect(
      page.getByRole("table", { name: "Dead letter queue" })
    ).toBeVisible();
  });
});
