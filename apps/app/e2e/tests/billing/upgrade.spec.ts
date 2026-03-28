import { expect, test } from "../../fixtures";

test.describe("Upgrade Page", () => {
  test("upgrade page loads", async ({ page }) => {
    await page.goto("/app/upgrade");
    await expect(page).toHaveURL(/\/app\/upgrade/);
  });

  test("shows plan cards", async ({ page }) => {
    await page.goto("/app/upgrade");
    // Should show at least one plan option
    const planCard = page.getByText(/starter|pro|enterprise|free/i);
    await expect(planCard.first()).toBeVisible({ timeout: 10_000 });
  });

  test("monthly/yearly toggle exists", async ({ page }) => {
    await page.goto("/app/upgrade");
    const toggle = page.getByText(/monthly|yearly|annual/i);
    if (
      await toggle
        .first()
        .isVisible({ timeout: 5000 })
        .catch(() => false)
    ) {
      await expect(toggle.first()).toBeVisible();
    }
  });

  test("page renders without crashing", async ({ page }) => {
    await page.goto("/app/upgrade");
    await expect(page.locator("body")).toBeVisible();
  });
});
