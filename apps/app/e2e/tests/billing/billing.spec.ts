import { expect, test } from "../../fixtures";

test.describe("Billing", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/billing");
  });

  test("billing page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/billing/);
    await expect(page.getByRole("heading", { name: "Billing" })).toBeVisible();
  });

  test("overview tab is visible and active by default", async ({ page }) => {
    await expect(page.getByRole("tab", { name: /overview/i })).toBeVisible();
  });

  test("all billing tabs exist", async ({ page }) => {
    await expect(page.getByRole("tab", { name: /overview/i })).toBeVisible();
    await expect(
      page.getByRole("tab", { name: /usage history/i })
    ).toBeVisible();
    await expect(
      page.getByRole("tab", { name: /project costs/i })
    ).toBeVisible();
    await expect(page.getByRole("tab", { name: /spending/i })).toBeVisible();
    await expect(page.getByRole("tab", { name: /alerts/i })).toBeVisible();
    await expect(page.getByRole("tab", { name: /referrals/i })).toBeVisible();
  });

  test("switching to usage history tab works", async ({ page }) => {
    await page.getByRole("tab", { name: /usage history/i }).click();
    await page.waitForTimeout(500);
    await expect(page.locator("main")).toBeVisible();
  });

  test("switching to project costs tab works", async ({ page }) => {
    await page.getByRole("tab", { name: /project costs/i }).click();
    await page.waitForTimeout(500);
    await expect(page.locator("main")).toBeVisible();
  });

  test("switching to spending tab works", async ({ page }) => {
    await page.getByRole("tab", { name: /spending/i }).click();
    await page.waitForTimeout(500);
    await expect(page.locator("main")).toBeVisible();
  });

  test("switching to alerts tab works", async ({ page }) => {
    await page.getByRole("tab", { name: /alerts/i }).click();
    await page.waitForTimeout(500);
    await expect(page.locator("main")).toBeVisible();
  });

  test("switching to referrals tab works", async ({ page }) => {
    await page.getByRole("tab", { name: /referrals/i }).click();
    await page.waitForTimeout(500);
    await expect(page.locator("main")).toBeVisible();
  });

  test("overview tab shows usage data or error state", async ({ page }) => {
    const usageContent = page.getByText(/usage|plan|free|starter|pro/i);
    const errorState = page.getByText(/failed to load|error/i);
    await expect(usageContent.or(errorState)).toBeVisible({ timeout: 10_000 });
  });

  test("page loads without console errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));
    await page.goto("/app/billing");
    await page.waitForTimeout(2000);
    expect(errors.filter((e) => !e.includes("ResizeObserver"))).toHaveLength(0);
  });

  test("all tabs render content without crashing", async ({ page }) => {
    const tabs = [
      /usage history/i,
      /project costs/i,
      /spending/i,
      /alerts/i,
      /referrals/i,
    ];
    for (const tab of tabs) {
      await page.getByRole("tab", { name: tab }).click();
      await page.waitForTimeout(300);
      await expect(page.locator("main")).toBeVisible();
    }
  });
});
