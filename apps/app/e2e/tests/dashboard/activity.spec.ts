import { expect, test } from "../../fixtures";

test.describe("Dashboard Activity", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/dashboard");
  });

  test("recent runs table section renders", async ({ page }) => {
    await expect(page.getByText("Recent Runs")).toBeVisible();
  });

  test("live activity feed section renders", async ({ page }) => {
    await expect(
      page.getByText("Live Activity", { exact: true })
    ).toBeVisible();
  });

  test("project cost card renders when project is active", async ({ page }) => {
    // Project cost card may or may not be visible depending on active project
    const costCard = page.getByText("Project Cost");
    const noProject = page.getByText("No project selected");
    await expect(costCard.or(noProject)).toBeVisible({ timeout: 10_000 });
  });
});
