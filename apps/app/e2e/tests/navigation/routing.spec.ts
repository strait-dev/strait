import { test, expect } from "../../fixtures";

test.describe("Routing", () => {
  test("deep link to dashboard works", async ({ page }) => {
    await page.goto("/app/dashboard");
    await expect(page).toHaveURL(/\/app\/dashboard/);
    await expect(page.getByText("Total Runs")).toBeVisible();
  });

  test("deep link to jobs works", async ({ page }) => {
    await page.goto("/app/jobs");
    await expect(page).toHaveURL(/\/app\/jobs/);
  });

  test("invalid route shows 404", async ({ page }) => {
    await page.goto("/app/nonexistent-page-12345");
    await expect(
      page.getByText(/not found|404|doesn't exist/i)
    ).toBeVisible({ timeout: 10_000 });
  });

  test("browser back navigation works", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.goto("/app/jobs");
    await expect(page).toHaveURL(/\/app\/jobs/);

    await page.goBack();
    await expect(page).toHaveURL(/\/app\/dashboard/);
  });
});
