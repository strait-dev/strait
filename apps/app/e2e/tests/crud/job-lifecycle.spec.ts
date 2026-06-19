import { ApiHelper, expect, test } from "../../fixtures";

const testJobName = `e2e-lifecycle-${Date.now()}`;

let api: ApiHelper;
let jobId: string;
let runId: string;

test.describe("Job Lifecycle", () => {
  test.describe.configure({ mode: "serial" });

  test.beforeAll(() => {
    api = new ApiHelper();
  });

  test.afterAll(async () => {
    if (jobId) {
      await api.deleteJob(jobId).catch(() => undefined);
    }
  });

  test("creates a job via the API", async () => {
    const job = await api.createJob({
      name: testJobName,
      endpoint_url: api.fakeEndpoint("/success"),
      max_attempts: 1,
      timeout_secs: 10,
    });

    jobId = job.id;
    expect(job.id).toBeTruthy();
    expect(job.name).toBe(testJobName);
  });

  test("shows the created job in the jobs list", async ({ page }) => {
    await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("region", { name: "Jobs" })).toBeVisible();
    await expect(page.getByText(testJobName)).toBeVisible({ timeout: 10_000 });
  });

  test("loads the job detail page", async ({ page }) => {
    await page.goto(`/app/jobs/${jobId}`, { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("heading", { name: testJobName })).toBeVisible({
      timeout: 10_000,
    });
    await expect(page.getByText("Configuration")).toBeVisible();
  });

  test("triggers the job and creates a run", async () => {
    const run = await api.triggerJob(jobId, { source: "job-lifecycle-e2e" });

    runId = run.id;
    expect(run.id).toBeTruthy();
  });

  test("shows the run in the runs list", async ({ page }) => {
    await page.goto("/app/runs", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("region", { name: "Runs" })).toBeVisible();
    await expect(page.getByText(runId.slice(0, 8)).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  test("deletes the job via the API", async () => {
    await api.waitForRunStatus(runId, ["completed", "failed"], 60_000);
    await api.deleteJob(jobId);
    jobId = "";
  });

  test("removes the deleted job from the jobs list", async ({ page }) => {
    await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("region", { name: "Jobs" })).toBeVisible();
    await expect(page.getByText(testJobName)).not.toBeVisible({
      timeout: 5000,
    });
  });
});
