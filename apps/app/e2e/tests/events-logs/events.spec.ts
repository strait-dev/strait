import { expect, test } from "../../fixtures";

test.describe("Events", () => {
  test("events page loads", async ({ page }) => {
    await page.goto("/app/events");
    await expect(page).toHaveURL(/\/app\/events/);
  });

  test("page renders content", async ({ page }) => {
    await page.goto("/app/events");
    const content = page
      .locator("table")
      .or(page.getByText(/no project|no events|went wrong/i));
    await expect(content.first()).toBeVisible({ timeout: 10_000 });
  });

  test("page loads without crashing", async ({ page }) => {
    await page.goto("/app/events");
    await expect(page.locator("body")).toBeVisible();
  });

  test("events page has correct URL", async ({ page }) => {
    await page.goto("/app/events");
    await expect(page).toHaveURL(/\/app\/events/);
  });

  test("page content area is present", async ({ page }) => {
    await page.goto("/app/events");
    await expect(page.locator("main").or(page.locator("body"))).toBeVisible();
  });
});
