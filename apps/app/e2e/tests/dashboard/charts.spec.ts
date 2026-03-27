import { test, expect } from "../../fixtures";

test.describe("Dashboard Charts", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/dashboard");
  });

  test("run activity chart renders", async ({ page }) => {
    await expect(page.getByText("Run Activity")).toBeVisible();
  });

  test("status distribution chart renders", async ({ page }) => {
    await expect(page.getByText("Status Distribution")).toBeVisible();
  });

  test("failed runs by job chart renders", async ({ page }) => {
    await expect(page.getByText("Failed Runs by Job")).toBeVisible();
  });

  test("throughput chart renders", async ({ page }) => {
    await expect(page.getByText("Throughput")).toBeVisible();
  });
});
