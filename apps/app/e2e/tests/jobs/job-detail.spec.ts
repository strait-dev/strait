import { ApiHelper, expect, test } from "../../fixtures";

const jobName = `e2e-core-detail-${Date.now()}`;

let api: ApiHelper;
let jobId: string;

test.describe("Job Detail", () => {
  test.beforeAll(async () => {
    api = new ApiHelper();
    const job = await api.createJob({
      name: jobName,
      endpoint_url: "https://httpbin.org/post",
      max_attempts: 3,
      timeout_secs: 20,
      description: "Detail job seeded by Playwright",
    });
    jobId = job.id;
  });

  test.afterAll(async () => {
    if (jobId) {
      await api.deleteJob(jobId).catch(() => undefined);
    }
  });

  test("overview shows status, actions, metrics, and configuration", async ({
    page,
  }) => {
    await page.goto(`/app/jobs/${jobId}`, { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("heading", { name: jobName })).toBeVisible();
    await expect(page.getByRole("button", { name: "Trigger" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Pause" })).toBeVisible();
    await expect(page.getByRole("button", { name: "1 hour" })).toBeVisible();
    await expect(page.getByRole("button", { name: "7 days" })).toBeVisible();
    await expect(page.getByText("Success Rate")).toBeVisible();
    await expect(page.getByText("Total Runs")).toBeVisible();
    await expect(page.getByText("Run Status Distribution")).toBeVisible();
    await expect(page.getByText("Configuration")).toBeVisible();
    await expect(page.getByText("https://httpbin.org/post")).toBeVisible();
  });

  test("time window selection keeps health cards visible", async ({ page }) => {
    await page.goto(`/app/jobs/${jobId}`, { waitUntil: "domcontentloaded" });

    await page.getByRole("button", { name: "30 days" }).click();
    await expect(page.getByRole("button", { name: "30 days" })).toBeVisible();
    await expect(page.getByText("Failed Runs")).toBeVisible();
  });
});
