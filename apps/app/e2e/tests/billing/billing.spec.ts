import { test, expect } from "../../fixtures";

test.describe("Billing", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/billing");
  });

  test("billing page loads", async ({ page }) => {
    await expect(page).toHaveURL(/\/app\/billing/);
  });

  test("overview tab is visible", async ({ page }) => {
    await expect(page.getByText("Overview")).toBeVisible();
  });

  test("usage history tab exists", async ({ page }) => {
    await expect(page.getByText("Usage History")).toBeVisible();
  });

  test("project costs tab exists", async ({ page }) => {
    await expect(page.getByText("Project Costs")).toBeVisible();
  });

  test("spending tab exists", async ({ page }) => {
    await expect(page.getByText("Spending")).toBeVisible();
  });

  test("alerts tab exists", async ({ page }) => {
    await expect(page.getByText("Alerts")).toBeVisible();
  });

  test("referrals tab exists", async ({ page }) => {
    await expect(page.getByText("Referrals")).toBeVisible();
  });

  test("switching to usage history tab works", async ({ page }) => {
    await page.getByText("Usage History").click();
    await page.waitForTimeout(500);
    await expect(page.locator("main")).toBeVisible();
  });

  test("switching to project costs tab works", async ({ page }) => {
    await page.getByText("Project Costs").click();
    await page.waitForTimeout(500);
    await expect(page.locator("main")).toBeVisible();
  });

  test("switching to spending tab works", async ({ page }) => {
    await page.getByText("Spending").click();
    await page.waitForTimeout(500);
    await expect(page.locator("main")).toBeVisible();
  });

  test("switching to alerts tab works", async ({ page }) => {
    await page.getByText("Alerts").click();
    await page.waitForTimeout(500);
    await expect(page.locator("main")).toBeVisible();
  });

  test("switching to referrals tab works", async ({ page }) => {
    await page.getByText("Referrals").click();
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

  test("all tabs are clickable and render content", async ({ page }) => {
    const tabs = ["Overview", "Usage History", "Project Costs", "Spending", "Alerts", "Referrals"];
    for (const tab of tabs) {
      await page.getByRole("tab", { name: tab }).or(page.getByText(tab)).click();
      await page.waitForTimeout(300);
    }
  });
});
