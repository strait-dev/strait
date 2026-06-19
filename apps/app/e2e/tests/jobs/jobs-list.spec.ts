import { ApiHelper, expect, test } from "../../fixtures";

const runId = Date.now();
const activeJobName = `e2e-core-active-${runId}`;
const pausedJobName = `e2e-core-paused-${runId}`;

let api: ApiHelper;
let activeJobId: string;
let pausedJobId: string;

test.describe("Jobs List", () => {
  test.beforeAll(async () => {
    api = new ApiHelper();
    const activeJob = await api.createJob({
      name: activeJobName,
      endpoint_url: api.fakeEndpoint("/success"),
      max_attempts: 2,
      timeout_secs: 15,
      description: "Active job seeded by Playwright",
    });
    const pausedJob = await api.createJob({
      name: pausedJobName,
      endpoint_url: api.fakeEndpoint("/success"),
      max_attempts: 1,
      timeout_secs: 10,
      description: "Paused job seeded by Playwright",
    });
    activeJobId = activeJob.id;
    pausedJobId = pausedJob.id;
    await api.pauseJob(pausedJobId);
  });

  test.afterAll(async () => {
    await Promise.allSettled(
      [activeJobId, pausedJobId].filter(Boolean).map((id) => api.deleteJob(id))
    );
  });

  test.beforeEach(async ({ page }) => {
    await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("region", { name: "Jobs" })).toBeVisible();
  });

  test("renders controls and seeded jobs", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/jobs/);
    await expect(page.getByPlaceholder("Search jobs")).toBeVisible();
    await expect(page.getByRole("button", { name: "Filter" })).toBeVisible();
    await expect(page.getByText(activeJobName)).toBeVisible();
    await expect(page.getByText(pausedJobName)).toBeVisible();
  });

  test("accepts search input without losing seeded rows", async ({ page }) => {
    await page.getByPlaceholder("Search jobs").fill(activeJobName);

    await expect(page.getByPlaceholder("Search jobs")).toHaveValue(
      activeJobName
    );
    await expect(page.getByText(activeJobName)).toBeVisible();
  });
});
