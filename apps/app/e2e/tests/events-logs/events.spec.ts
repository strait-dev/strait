import { expect, test } from "../../fixtures";

test.describe("Events", () => {
  test("events page loads", async ({ page }) => {
    await page.goto("/app/events");
    await expect(page.locator("main")).toBeVisible();
  });

  test("events table or empty state is visible", async ({ page }) => {
    await page.goto("/app/events");
    const table = page.locator("table");
    const emptyState = page.getByText(/no events|no project/i);
    await expect(table.or(emptyState)).toBeVisible();
  });

  test("page loads without console errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));
    await page.goto("/app/events");
    await page.waitForTimeout(2000);
    expect(errors.filter((e) => !e.includes("ResizeObserver"))).toHaveLength(0);
  });

  test("events page has correct URL", async ({ page }) => {
    await page.goto("/app/events");
    await expect(page).toHaveURL(/\/app\/events/);
  });

  test("page content area is present", async ({ page }) => {
    await page.goto("/app/events");
    await expect(page.locator("main")).toBeVisible();
  });
});
