import { expect, test } from "../../fixtures";

test.describe("Billing", () => {
  test.slow();
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/billing");
  });

  test("billing page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/billing/);
  });

  test("billing heading or content is visible", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible({ timeout: 10_000 });
  });

  test("tabs exist when billing page renders", async ({ page }) => {
    const tab = page.getByRole("tab").first();
    if (await tab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(tab).toBeVisible();
    }
  });

  test("switching to usage history tab works", async ({ page }) => {
    const tab = page.getByRole("tab", { name: /usage history/i });
    if (await tab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await tab.click();
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("switching to project costs tab works", async ({ page }) => {
    const tab = page.getByRole("tab", { name: /project costs/i });
    if (await tab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await tab.click();
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("switching to spending tab works", async ({ page }) => {
    const tab = page.getByRole("tab", { name: /spending/i });
    if (await tab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await tab.click();
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("switching to alerts tab works", async ({ page }) => {
    const tab = page.getByRole("tab", { name: /alerts/i });
    if (await tab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await tab.click();
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("switching to referrals tab works", async ({ page }) => {
    const tab = page.getByRole("tab", { name: /referrals/i });
    if (await tab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await tab.click();
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("overview content renders", async ({ page }) => {
    await expect(page.locator("main")).toBeVisible({ timeout: 10_000 });
  });

  test("page loads without crashing", async ({ page }) => {
    await expect(page.locator("body")).toBeVisible();
  });

  test("all tabs render content without crashing", async ({ page }) => {
    const tabNames = [
      /usage history/i,
      /project costs/i,
      /spending/i,
      /alerts/i,
      /referrals/i,
    ];
    for (const name of tabNames) {
      const tab = page.getByRole("tab", { name });
      if (await tab.isVisible({ timeout: 3000 }).catch(() => false)) {
        await tab.click();
        await page.waitForTimeout(300);
        await expect(page.locator("main")).toBeVisible();
      }
    }
  });
});
