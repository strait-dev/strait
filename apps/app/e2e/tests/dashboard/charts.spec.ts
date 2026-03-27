import { expect, test } from "../../fixtures";

test.describe("Dashboard Charts", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/dashboard");
  });

  test("run activity chart renders", async ({ page }) => {
    await expect(page.getByText("Run Activity", { exact: true })).toBeVisible();
  });

  test("status distribution chart renders", async ({ page }) => {
    await expect(
      page.getByText("Status Distribution", { exact: true })
    ).toBeVisible();
  });

  test("failed runs by job chart renders", async ({ page }) => {
    await expect(
      page.getByText("Failed Runs by Job", { exact: true })
    ).toBeVisible();
  });

  test("throughput chart renders", async ({ page }) => {
    await expect(page.getByText("Throughput (24h)")).toBeVisible();
  });
});
