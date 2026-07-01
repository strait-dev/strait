import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

let data: TestDataFactory;
let prefix: string;

test.describe("Job filter combinations", () => {
  test.beforeAll(async () => {
    const api = new ApiHelper();
    data = new TestDataFactory(api);
    prefix = data.name("filter");

    for (let index = 0; index < 5; index += 1) {
      await data.job(`${prefix}-${index}`, { name: `${prefix}-${index}` });
    }
    await expect(async () => {
      const jobs = await api.listJobs({ limit: 10, search: prefix });
      expect(jobs.data.some((job) => job.name === `${prefix}-0`)).toBe(true);
    }).toPass({ timeout: 15_000 });
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("search filters seeded jobs and can return to all results", async ({
    page,
  }) => {
    await page.goto(`/app/jobs?query=${encodeURIComponent(prefix)}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(page.getByRole("region", { name: "Jobs" })).toBeVisible();
    await expect(page.getByText(`${prefix}-0`)).toBeVisible();

    await page.goto("/app/jobs?query=missing-job-xyz", {
      waitUntil: "domcontentloaded",
    });
    await expect(page.getByText(`${prefix}-0`)).not.toBeVisible();
    await expect(page.getByText("0 rows")).toBeVisible();

    await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("region", { name: "Jobs" })).toBeVisible();
  });

  test("status filter exposes selectable job status options", async ({
    page,
  }) => {
    await page.goto("/app/jobs?status=Enabled", {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.getByRole("button", { name: /Status\s+1/ })
    ).toBeVisible();
    await expect(page.getByText(`${prefix}-0`)).toBeVisible();
  });
});
