import { ApiHelper, expect, test } from "../../fixtures";
import { selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

test.describe("Run detail page", () => {
  test.describe.configure({ timeout: 120_000 });
  test.setTimeout(120_000);

  test.beforeEach(({ browserName }, testInfo) => {
    testInfo.annotations.push({ type: "browser", description: browserName });
    testInfo.setTimeout(120_000);
  });

  test("renders run metadata, payload, response, and logs", async ({
    page,
  }) => {
    const api = new ApiHelper();
    const data = new TestDataFactory(api);
    const job = await data.job("run-detail");
    const run = await api.triggerJob(job.id, { expected: "success" });

    try {
      await expect(async () => {
        const found = await api.getRun(run.id);
        expect(found.id).toBe(run.id);
      }).toPass({ timeout: 15_000 });

      await page.goto(`/app/runs/${run.id}`, {
        waitUntil: "domcontentloaded",
      });

      await expect(page.getByRole("heading", { name: run.id })).toBeVisible();
      await expect(
        page
          .getByText(/queued|executing|completed|succeeded|failed|dead letter/i)
          .first()
      ).toBeVisible();
      await expect(page.getByText("What happened")).toBeVisible();

      await selectTab(page, "Payload");
      await expect(page.getByText("expected")).toBeVisible();
      await selectTab(page, "Response");
      await expect(page.locator("pre").first()).toBeVisible();
      await selectTab(page, "Logs");
      await expect(page.getByText(/No log events|run/i).first()).toBeVisible();
    } finally {
      await api.cancelRun(run.id).catch(() => undefined);
      await data.cleanup.run();
    }
  });

  test("renders failed run context for dead-lettered runs", async ({
    page,
  }) => {
    const api = new ApiHelper();
    const data = new TestDataFactory(api);
    const failed = await data.failedJobRun("run-detail-failed");
    const failedRunId = failed.run.id;

    try {
      await page.goto(`/app/runs/${failedRunId}`, {
        waitUntil: "domcontentloaded",
      });

      await expect(
        page.getByRole("heading", { name: failedRunId })
      ).toBeVisible();
      await expect(page.getByText(/failed|dead letter/i).first()).toBeVisible();
      await expect(page.getByText("What happened")).toBeVisible();
      await selectTab(page, "Payload");
      await expect(page.getByText("expected")).toBeVisible();
      await selectTab(page, "Response");
      await expect(page.locator("pre").first()).toBeVisible();
    } finally {
      await data.cleanup.run();
    }
  });

  test("shows not-found state for invalid run IDs", async ({ page }) => {
    await page.goto("/app/runs/not-a-real-run", {
      waitUntil: "domcontentloaded",
    });

    await expect(page.locator("main")).toBeVisible();
    await expect(
      page.getByText(/not found|couldn't find|error/i)
    ).toBeVisible();
  });
});
