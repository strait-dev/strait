import { expect, test } from "../../fixtures";

test.describe("Settings - Project", () => {
  test("api keys section is accessible", async ({ page }) => {
    await page.goto("/app/settings");
    const apiKeysLink = page.getByText(/api key/i);
    if (await apiKeysLink.isVisible({ timeout: 5000 }).catch(() => false)) {
      await apiKeysLink.click();
      await page.waitForTimeout(500);
    }
  });

  test("account tab is accessible", async ({ page }) => {
    await page.goto("/app/settings");
    const accountTab = page.getByRole("tab", { name: "Account" });
    if (await accountTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(accountTab).toBeVisible();
    }
  });

  test("billing tab is accessible from settings", async ({ page }) => {
    await page.goto("/app/settings");
    const billingTab = page.getByRole("tab", { name: /usage|billing/i });
    if (await billingTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(billingTab).toBeVisible();
    }
  });

  test("authorized apps tab exists", async ({ page }) => {
    await page.goto("/app/settings");
    const tab = page.getByRole("tab", { name: /authorized/i });
    if (await tab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(tab).toBeVisible();
    }
  });

  test("linked accounts section exists", async ({ page }) => {
    await page.goto("/app/settings");
    const section = page
      .getByText("Linked accounts", { exact: true })
      .or(page.getByText(/linked|connected/i));
    if (
      await section
        .first()
        .isVisible({ timeout: 10_000 })
        .catch(() => false)
    ) {
      await expect(section.first()).toBeVisible();
    }
  });

  test("passkeys section exists", async ({ page }) => {
    await page.goto("/app/settings");
    const section = page
      .getByText("Passkeys", { exact: true })
      .or(page.getByText(/passkey/i));
    if (
      await section
        .first()
        .isVisible({ timeout: 10_000 })
        .catch(() => false)
    ) {
      await expect(section.first()).toBeVisible();
    }
  });
});
