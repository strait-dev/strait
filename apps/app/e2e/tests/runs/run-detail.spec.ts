import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

let data: TestDataFactory;
let runId: string;

test.describe("Runs list to detail navigation", () => {
  test.describe.configure({ timeout: 90_000 });

  test.beforeAll(async ({ browserName: _browserName }, testInfo) => {
    testInfo.setTimeout(90_000);
    const api = new ApiHelper();
    data = new TestDataFactory(api);
    const seeded = await data.successfulJobRun("runs-list-detail", 60_000);
    runId = seeded.run.id;
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("filters to a seeded run and opens the detail page", async ({
    page,
  }) => {
    await page.goto("/app/runs", { waitUntil: "domcontentloaded" });

    await page.getByLabel("Search").fill(runId.slice(0, 8));
    const runLink = page.getByRole("link", { name: runId.slice(0, 8) }).first();
    await expect(runLink).toBeVisible({ timeout: 15_000 });
    await runLink.click();

    await expect(page).toHaveURL(new RegExp(`/app/runs/${runId}`));
    await expect(page.getByRole("heading", { name: runId })).toBeVisible();
    await expect(page.getByText(/completed|succeeded/i).first()).toBeVisible();
  });
});
