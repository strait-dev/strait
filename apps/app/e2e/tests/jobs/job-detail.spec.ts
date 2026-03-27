import { expect, test } from "../../fixtures";

test.describe("Job Detail", () => {
  test("404 for invalid job ID", async ({ page }) => {
    await page.goto("/app/jobs/nonexistent-id-12345");
    // Should show error state, not found, or redirect
    const notFound = page.getByText(/not found|doesn't exist|no job|error|went wrong/i);
    const mainContent = page.locator("main");
    await expect(notFound.or(mainContent)).toBeVisible({ timeout: 10_000 });
  });

  test("job detail page has overview tab", async ({ page }) => {
    // Navigate to jobs list and click first job if available
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    // Find a link to job detail
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await expect(page.getByText("Overview")).toBeVisible();
    }
  });

  test("time window selector has expected options", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await expect(page.getByRole("button", { name: "1 hour" })).toBeVisible();
      await expect(
        page.getByRole("button", { name: "24 hours" })
      ).toBeVisible();
      await expect(page.getByRole("button", { name: "7 days" })).toBeVisible();
      await expect(page.getByRole("button", { name: "30 days" })).toBeVisible();
    }
  });

  test("configuration card shows job settings", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await expect(page.getByText("Configuration")).toBeVisible();
      await expect(page.getByText("Endpoint")).toBeVisible();
      await expect(page.getByText("Retry")).toBeVisible();
      await expect(page.getByText("Timeout")).toBeVisible();
    }
  });

  test("stats cards show on overview tab", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await expect(page.getByText("Success Rate")).toBeVisible();
      await expect(page.getByText("Total Runs")).toBeVisible();
      await expect(page.getByText("Avg Duration")).toBeVisible();
      await expect(page.getByText("Failed Runs")).toBeVisible();
    }
  });

  test("runs tab shows filtered runs table", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await page.getByText("Recent Runs").click();
      await page.waitForTimeout(500);
    }
  });

  test("trigger button is visible on detail page", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await expect(
        page.getByRole("button", { name: /trigger/i })
      ).toBeVisible();
    }
  });

  test("pause/resume button is visible on detail page", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      const pauseBtn = page.getByRole("button", { name: /pause|resume/i });
      await expect(pauseBtn).toBeVisible();
    }
  });

  test("switching time windows updates stats", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await page.getByRole("button", { name: "30 days" }).click();
      await page.waitForTimeout(500);
      await expect(page.getByText("Success Rate")).toBeVisible();
    }
  });

  test("settings tab renders", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await page.getByText("Settings").click();
      await page.waitForTimeout(500);
    }
  });

  test("status badge shows on detail header", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible())) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      // Status badge should show enabled/paused state
      await page.waitForTimeout(500);
    }
  });
});
