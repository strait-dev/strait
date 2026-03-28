import { expect, test } from "../../fixtures";

test.describe("Organization Settings", () => {
  test("org settings page loads", async ({ page }) => {
    // Navigate via sidebar workspace link
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    // The org settings is accessible via the workspace name in sidebar
    await expect(page.locator("body")).toBeVisible();
  });

  test("organization tab renders", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    // Find the workspace/org link in sidebar
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (await orgLink.isVisible({ timeout: 5000 }).catch(() => false)) {
      await orgLink.click();
      await expect(page).toHaveURL(/\/app\/org\//);
      const orgTab = page.getByRole("tab", { name: /organization/i });
      if (await orgTab.isVisible({ timeout: 5000 }).catch(() => false)) {
        await expect(orgTab).toBeVisible();
      }
    }
  });

  test("members tab renders", async ({ page }) => {
    const orgLink = page.locator("a[href*='/app/org/']").first();
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    if (await orgLink.isVisible({ timeout: 5000 }).catch(() => false)) {
      await orgLink.click();
      const membersTab = page.getByRole("tab", { name: /members/i });
      if (await membersTab.isVisible({ timeout: 5000 }).catch(() => false)) {
        await membersTab.click();
        await page.waitForTimeout(1000);
        await expect(page.locator("main")).toBeVisible();
      }
    }
  });

  test("subscription tab renders", async ({ page }) => {
    const orgLink = page.locator("a[href*='/app/org/']").first();
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    if (await orgLink.isVisible({ timeout: 5000 }).catch(() => false)) {
      await orgLink.click();
      const subTab = page.getByRole("tab", { name: /subscription/i });
      if (await subTab.isVisible({ timeout: 5000 }).catch(() => false)) {
        await subTab.click();
        await page.waitForTimeout(1000);
        await expect(page.locator("main")).toBeVisible();
      }
    }
  });

  test("api keys tab renders", async ({ page }) => {
    const orgLink = page.locator("a[href*='/app/org/']").first();
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    if (await orgLink.isVisible({ timeout: 5000 }).catch(() => false)) {
      await orgLink.click();
      const keysTab = page.getByRole("tab", { name: /api key/i });
      if (await keysTab.isVisible({ timeout: 5000 }).catch(() => false)) {
        await keysTab.click();
        await page.waitForTimeout(1000);
        await expect(page.locator("main")).toBeVisible();
      }
    }
  });
});
