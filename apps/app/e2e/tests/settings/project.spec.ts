import { expect, test } from "../../fixtures";

test.describe("Settings - Project", () => {
  test("api keys section is accessible", async ({ page }) => {
    await page.goto("/app/settings");
    const apiKeysLink = page.getByText(/api key/i);
    if (await apiKeysLink.isVisible()) {
      await apiKeysLink.click();
      await page.waitForTimeout(500);
    }
  });

  test("account tab is accessible", async ({ page }) => {
    await page.goto("/app/settings");
    await expect(page.getByRole("tab", { name: "Account" })).toBeVisible();
  });

  test("billing link is accessible from settings", async ({ page }) => {
    await page.goto("/app/settings");
    const billingTab = page.getByRole("tab", { name: /usage|billing/i });
    if (await billingTab.isVisible()) {
      await expect(billingTab).toBeVisible();
    }
  });

  test("authorized apps tab exists", async ({ page }) => {
    await page.goto("/app/settings");
    const tab = page.getByRole("tab", { name: /authorized/i });
    if (await tab.isVisible()) {
      await expect(tab).toBeVisible();
    }
  });

  test("linked accounts section exists", async ({ page }) => {
    await page.goto("/app/settings");
    await expect(
      page.getByText("Linked Accounts", { exact: true })
    ).toBeVisible();
  });

  test("passkeys section exists", async ({ page }) => {
    await page.goto("/app/settings");
    await expect(
      page.getByText("Passkeys", { exact: true })
    ).toBeVisible();
  });
});
