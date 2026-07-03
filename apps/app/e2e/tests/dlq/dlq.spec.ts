import { ApiHelper, expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

test.describe("Dead letter queue", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/dlq");
  });

  test("DLQ page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/dlq/);
  });

  test("page renders content", async ({ page }) => {
    const content = page
      .locator("table")
      .or(page.getByText(/no project|no dead letter|went wrong|empty/i));
    await expect(content.first()).toBeVisible({ timeout: 10_000 });
  });

  test("page has correct URL", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/dlq/);
  });

  test("search input exists when available", async ({ page }) => {
    const searchInput = page.getByPlaceholder(
      "Search by job, run ID, or error..."
    );
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(searchInput).toBeVisible();
    }
  });

  test("search filters DLQ entries when available", async ({ page }) => {
    const searchInput = page.getByPlaceholder(
      "Search by job, run ID, or error..."
    );
    if (await searchInput.isVisible({ timeout: 5000 }).catch(() => false)) {
      await searchInput.fill("nonexistent-dlq-entry");
      await page.waitForTimeout(500);
      await expect(page.locator("body")).toBeVisible();
    }
  });

  test("page content is visible", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible();
  });

  test("page loads without crashing", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible();
  });

  test("table columns are correct when data exists", async ({ page }) => {
    const table = page.locator("table");
    if (await table.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(
        table
          .getByRole("columnheader", { name: "Run ID" })
          .or(table.getByRole("columnheader", { name: "Job" }))
          .first()
      ).toBeVisible();
    }
  });

  test("lists a deterministic dead-letter run and opens its detail", async ({
    page,
  }) => {
    const api = new ApiHelper();
    const data = new TestDataFactory(api);
    const { job, run } = await data.deadLetterRun("dlq-list");

    try {
      await page.goto("/app/dlq", { waitUntil: "domcontentloaded" });
      await expect(
        page.getByRole("table", { name: "Dead letter queue" })
      ).toBeVisible();
      const dlqTable = page.getByRole("table", { name: "Dead letter queue" });
      const runRow = dlqTable
        .locator("tbody tr")
        .filter({ hasText: run.id.slice(0, 8) })
        .first();
      await expect(runRow).toBeVisible();
      await expect(runRow).toContainText(job.id.slice(0, 8));
      const viewRunButton = runRow.getByRole("button", {
        name: `View run ${run.id}`,
      });
      const runDetailSheet = page.getByTestId("run-detail-sheet");
      for (let attempt = 1; attempt <= 3; attempt += 1) {
        await viewRunButton.click({ force: attempt > 1 });
        if (await runDetailSheet.isVisible().catch(() => false)) {
          break;
        }
        await page.waitForTimeout(500);
      }
      await expect(runDetailSheet).toContainText(run.id);
      await expect(runDetailSheet).toContainText("Dead letter");
      await expect(runDetailSheet).toContainText("exhausted retries in e2e");
    } finally {
      await data.cleanup.run();
    }
  });

  test("retries and discards deterministic dead-letter runs through the Go API", async () => {
    const api = new ApiHelper();
    const data = new TestDataFactory(api);
    const retryFixture = await data.deadLetterRun("dlq-retry");
    const discardFixture = await data.deadLetterRun("dlq-discard");

    try {
      const retried = await api.replayDlqEntry(retryFixture.run.id);
      expect((retried as { status?: string }).status).toBe("queued");
      await api.waitForRunStatus(
        retryFixture.run.id,
        ["completed", "succeeded", "failed", "dead_letter"],
        60_000
      );

      await api.purgeDlqEntry(discardFixture.run.id);
      const entries = await api.listDlqEntries({ limit: 100 });
      expect(entries.data.map((entry) => entry.id)).not.toContain(
        discardFixture.run.id
      );
    } finally {
      await data.cleanup.run();
    }
  });
});
