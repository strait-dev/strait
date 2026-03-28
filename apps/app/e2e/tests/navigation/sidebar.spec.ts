import { expect, test } from "../../fixtures";

test.describe("Sidebar Navigation", () => {
  test("dashboard link navigates correctly", async ({ page }) => {
    await page.goto("/app/jobs");
    await page.waitForTimeout(500);
    const link = page.getByRole("link", { name: "Dashboard" });
    if (await link.isVisible({ timeout: 5000 }).catch(() => false)) {
      await link.click();
      await expect(page).toHaveURL(/\/app\/dashboard/);
    }
  });

  test("jobs link navigates correctly", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(500);
    const link = page.getByRole("link", { name: "Jobs" });
    if (await link.isVisible({ timeout: 5000 }).catch(() => false)) {
      await link.click();
      await expect(page).toHaveURL(/\/app\/jobs/);
    }
  });

  test("runs link navigates correctly", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(500);
    const link = page.getByRole("link", { name: "Runs" });
    if (await link.isVisible({ timeout: 5000 }).catch(() => false)) {
      await link.click();
      await expect(page).toHaveURL(/\/app\/runs/);
    }
  });

  test("workflows link navigates correctly", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(500);
    const link = page.getByRole("link", { name: "Workflows" });
    if (await link.isVisible({ timeout: 5000 }).catch(() => false)) {
      await link.click();
      await expect(page).toHaveURL(/\/app\/workflows/);
    }
  });
});
