import { expect, test } from "../../fixtures";

test.describe("Dashboard Smoke", () => {
  test("dashboard page loads for authenticated user", async ({ page }) => {
    await page.goto("/app/dashboard");

    await expect(page).toHaveURL(/\/app\/dashboard/);
    await expect(page.locator("main")).toBeVisible();
  });

  test("metrics cards are visible", async ({ page }) => {
    await page.goto("/app/dashboard");

    await expect(page.getByText("Total runs (24h)")).toBeVisible();
    await expect(page.getByText("Success rate")).toBeVisible();
    await expect(page.getByText("Failed runs", { exact: true })).toBeVisible();
    await expect(page.getByText("Queued")).toBeVisible();
  });

  test("run activity chart section renders", async ({ page }) => {
    await page.goto("/app/dashboard");

    await expect(page.getByText("Run activity", { exact: true })).toBeVisible();
  });

  test("status distribution chart section renders", async ({ page }) => {
    await page.goto("/app/dashboard");

    await expect(
      page.getByText("Status distribution", { exact: true })
    ).toBeVisible();
  });

  test("recent runs table and live activity feed render", async ({ page }) => {
    await page.goto("/app/dashboard");

    await expect(page.getByText("Recent runs", { exact: true })).toBeVisible();
    await expect(
      page.getByText("Live activity", { exact: true })
    ).toBeVisible();
  });
});
