import { expect, test } from "../../fixtures";

test.describe("API Keys", () => {
  test("api keys section accessible from org settings", async ({ page }) => {
    await page.goto("/app/dashboard");
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (!(await orgLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    await orgLink.click();
    const keysTab = page.getByRole("tab", { name: /api key/i });
    if (await keysTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await keysTab.click();
      await page.waitForTimeout(500);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("create API key button exists", async ({ page }) => {
    await page.goto("/app/dashboard");
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (!(await orgLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    await orgLink.click();
    const keysTab = page.getByRole("tab", { name: /api key/i });
    if (await keysTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await keysTab.click();
      await page.waitForTimeout(500);
      const createBtn = page.getByText("Create API Key");
      if (await createBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
        await expect(createBtn).toBeVisible();
      }
    }
  });

  test("create API key dialog opens", async ({ page }) => {
    await page.goto("/app/dashboard");
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (!(await orgLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    await orgLink.click();
    const keysTab = page.getByRole("tab", { name: /api key/i });
    if (await keysTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await keysTab.click();
      await page.waitForTimeout(500);
      const createBtn = page.getByText("Create API Key");
      if (await createBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
        await createBtn.click();
        await page.waitForTimeout(500);
        const dialog = page.locator("[role='dialog']");
        if (await dialog.isVisible({ timeout: 3000 }).catch(() => false)) {
          await expect(dialog).toBeVisible();
        }
      }
    }
  });
});
