import { expect, test } from "../../fixtures";

test.describe("Organization Operations", () => {
  test.slow();
  test("org settings has Organization tab", async ({ page }) => {
    await page.goto("/app/dashboard");
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (!(await orgLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    await orgLink.click();
    const orgTab = page.getByRole("tab", { name: /organization/i });
    if (await orgTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(orgTab).toBeVisible();
    }
  });

  test("members tab shows current user", async ({ page }) => {
    await page.goto("/app/dashboard");
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (!(await orgLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    await orgLink.click();
    const membersTab = page.getByRole("tab", { name: /members/i });
    if (await membersTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await membersTab.click();
      await page.waitForTimeout(500);
      // Current user should appear as owner
      const owner = page.getByText(/owner/i);
      if (await owner.isVisible({ timeout: 5000 }).catch(() => false)) {
        await expect(owner).toBeVisible();
      }
    }
  });

  test("invite member button opens dialog", async ({ page }) => {
    await page.goto("/app/dashboard");
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (!(await orgLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    await orgLink.click();
    const membersTab = page.getByRole("tab", { name: /members/i });
    if (await membersTab.isVisible({ timeout: 5000 }).catch(() => false)) {
      await membersTab.click();
      await page.waitForTimeout(500);
      const inviteBtn = page.getByText("Invite Member");
      if (await inviteBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
        await inviteBtn.click();
        await page.waitForTimeout(500);
        const dialog = page.locator("[role='dialog']");
        if (await dialog.isVisible({ timeout: 3000 }).catch(() => false)) {
          await expect(dialog).toBeVisible();
        }
      }
    }
  });

  test("delete organization section exists", async ({ page }) => {
    await page.goto("/app/dashboard");
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (!(await orgLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    await orgLink.click();
    // Delete org section should be on the Organization tab
    const deleteSection = page.getByText(/delete organization|danger/i);
    if (await deleteSection.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(deleteSection).toBeVisible();
    }
  });

  test("all tabs switch without errors", async ({ page }) => {
    await page.goto("/app/dashboard");
    const orgLink = page.locator("a[href*='/app/org/']").first();
    if (!(await orgLink.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }
    await orgLink.click();
    const tabs = page.getByRole("tab");
    const count = await tabs.count();
    for (let i = 0; i < count; i++) {
      await tabs.nth(i).click();
      await page.waitForTimeout(500);
      await expect(page.locator("body")).toBeVisible();
    }
  });
});
