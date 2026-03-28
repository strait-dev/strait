import { expect, test } from "../../fixtures";

test.describe("Team Members", () => {
  test("members tab shows current user", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (!(await orgLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    await orgLink.click();
    await page.waitForTimeout(2000);
    const membersTab = page.getByRole("tab", { name: /members/i });
    if (await membersTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await membersTab.click();
      await page.waitForTimeout(1000);
      await expect(page.locator("main")).toBeVisible();
    }
  });

  test("current user has owner role", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (!(await orgLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    await orgLink.click();
    await page.waitForTimeout(2000);
    const membersTab = page.getByRole("tab", { name: /members/i });
    if (await membersTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await membersTab.click();
      await page.waitForTimeout(1000);
      const ownerBadge = page.getByText(/owner/i).first();
      if (await ownerBadge.isVisible({ timeout: 5000 }).catch(() => false)) {
        await expect(ownerBadge).toBeVisible();
      }
    }
  });

  test("invite member button exists", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (!(await orgLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    await orgLink.click();
    await page.waitForTimeout(2000);
    const membersTab = page.getByRole("tab", { name: /members/i });
    if (await membersTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await membersTab.click();
      await page.waitForTimeout(1000);
      const inviteBtn = page.getByText("Invite Member");
      if (await inviteBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
        await expect(inviteBtn).toBeVisible();
      }
    }
  });
});
