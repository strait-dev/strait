import { ApiHelper, expect, test } from "../../fixtures";

const jobName = `e2e-core-detail-${Date.now()}`;

let api: ApiHelper;
let jobId: string;
let endpointUrl: string;

test.describe("Job Detail", () => {
  test.beforeAll(async () => {
    api = new ApiHelper();
    endpointUrl = api.fakeEndpoint("/success");
    const job = await api.createJob({
      name: jobName,
      endpoint_url: endpointUrl,
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
    await expect(page.getByText("Success rate")).toBeVisible();
    await expect(page.getByText("Total runs")).toBeVisible();
    await expect(page.getByText("Run status distribution")).toBeVisible();
    await expect(page.getByText("Configuration")).toBeVisible();
    await expect(page.getByText(endpointUrl)).toBeVisible();
  });

  test("time window selection keeps health cards visible", async ({ page }) => {
    await page.goto(`/app/jobs/${jobId}`, { waitUntil: "domcontentloaded" });

    await page.getByRole("button", { name: "30 days" }).click();
    await expect(page.getByRole("button", { name: "30 days" })).toBeVisible();
    await expect(page.getByText("Failed runs")).toBeVisible();
  });
});
