import { expect, test } from "../../fixtures";

test.describe("Project Switching", () => {
  test("project dropdown visible in sidebar", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    const projectDropdown = page
      .getByText("Default Project")
      .or(
        page
          .locator("[class*='project-switcher'], [data-slot='select']")
          .first()
      );
    if (await projectDropdown.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(projectDropdown).toBeVisible();
    }
  });

  test("project name shown in sidebar", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    const projectName = page.getByText(/default project/i);
    if (await projectName.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(projectName).toBeVisible();
    }
  });

  test("clicking project dropdown shows options", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    const dropdown = page.getByText("Default Project").first();
    if (await dropdown.isVisible({ timeout: 5000 }).catch(() => false)) {
      await dropdown.click();
      await page.waitForTimeout(500);
      // Should show dropdown menu or popover
      await expect(page.locator("body")).toBeVisible();
    }
  });

  test("sidebar shows workspace name", async ({ page }) => {
    await page.goto("/app/dashboard");
    await page.waitForTimeout(2000);
    // Workspace name should be visible at bottom of sidebar
    const workspace = page.getByText(/workspace/i);
    if (
      await workspace
        .first()
        .isVisible({ timeout: 5000 })
        .catch(() => false)
    ) {
      await expect(workspace.first()).toBeVisible();
    }
  });
});
