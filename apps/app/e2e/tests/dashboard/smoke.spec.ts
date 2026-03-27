import { test, expect } from "../../fixtures";

test.describe("Dashboard Smoke", () => {
  test("dashboard page loads for authenticated user", async ({ page }) => {
    await page.goto("/app/dashboard");

    await expect(page).toHaveURL(/\/app\/dashboard/);
    await expect(page.locator("main")).toBeVisible();
  });

  test("metrics cards are visible", async ({ page }) => {
    await page.goto("/app/dashboard");

    await expect(page.getByText("Total Runs (24h)")).toBeVisible();
    await expect(page.getByText("Success Rate")).toBeVisible();
    await expect(page.getByText("Failed Runs")).toBeVisible();
    await expect(page.getByText("Queued")).toBeVisible();
  });

  test("run activity chart section renders", async ({ page }) => {
    await page.goto("/app/dashboard");

    await expect(page.getByText("Run Activity")).toBeVisible();
  });

  test("status distribution chart section renders", async ({ page }) => {
    await page.goto("/app/dashboard");

    await expect(page.getByText("Status Distribution")).toBeVisible();
  });

  test("recent runs table and live activity feed render", async ({
    page,
  }) => {
    await page.goto("/app/dashboard");

    await expect(page.getByText("Recent Runs")).toBeVisible();
    await expect(page.getByText("Live Activity")).toBeVisible();
  });
});
