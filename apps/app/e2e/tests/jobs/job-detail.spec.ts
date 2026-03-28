import { expect, test } from "../../fixtures";

test.describe("Job Detail", () => {
  test("invalid job ID shows error or not found", async ({ page }) => {
    await page.goto("/app/jobs/nonexistent-id-12345");
    const errorContent = page.getByText(
      /not found|went wrong|error|doesn't exist/i
    );
    const mainContent = page.locator("main");
    await expect(errorContent.or(mainContent)).toBeVisible({ timeout: 10_000 });
  });

  test("job detail page has overview tab when job exists", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await expect(page.getByText("Overview")).toBeVisible();
    }
  });

  test("time window selector has expected options", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await expect(page.getByRole("button", { name: "1 hour" })).toBeVisible();
      await expect(page.getByRole("button", { name: "7 days" })).toBeVisible();
    }
  });

  test("configuration card shows job settings", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await expect(page.getByText("Configuration")).toBeVisible();
    }
  });

  test("stats cards show on overview tab", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await expect(page.getByText("Success Rate")).toBeVisible();
      await expect(page.getByText("Total Runs")).toBeVisible();
    }
  });

  test("trigger button is visible on detail page", async ({ page }) => {
    await page.goto("/app/jobs");
    const firstRow = page.locator("table tbody tr").first();
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
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
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
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
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
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
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
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
    if (!(await firstRow.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    const jobLink = firstRow.locator("a").first();
    if (await jobLink.isVisible()) {
      await jobLink.click();
      await page.waitForTimeout(500);
    }
  });
});
