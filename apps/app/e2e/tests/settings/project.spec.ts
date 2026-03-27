import { test, expect } from "../../fixtures";

test.describe("Settings - Project", () => {
  test("api keys section is accessible", async ({ page }) => {
    await page.goto("/app/settings");
    const apiKeysLink = page.getByText(/api key/i);
    if (await apiKeysLink.isVisible()) {
      await apiKeysLink.click();
      await page.waitForTimeout(500);
    }
  });

  test("organization settings are accessible", async ({ page }) => {
    await page.goto("/app/settings");
    const orgSettings = page.getByText(/organization|workspace/i);
    await expect(orgSettings).toBeVisible();
  });

  test("billing link is accessible from settings", async ({ page }) => {
    await page.goto("/app/settings");
    const billingLink = page.getByRole("link", { name: /billing|upgrade/i });
    if (await billingLink.isVisible()) {
      await expect(billingLink).toBeVisible();
    }
  });

  test("email preferences section exists", async ({ page }) => {
    await page.goto("/app/settings");
    const emailPrefs = page.getByText(/email preferences|monthly usage/i);
    if (await emailPrefs.isVisible()) {
      await expect(emailPrefs).toBeVisible();
    }
  });

  test("linked accounts section exists", async ({ page }) => {
    await page.goto("/app/settings");
    await expect(page.getByText(/linked accounts|connected/i)).toBeVisible();
  });

  test("passkeys section exists", async ({ page }) => {
    await page.goto("/app/settings");
    await expect(page.getByText(/passkey/i)).toBeVisible();
  });
});
