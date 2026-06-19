import { expect, test } from "../../fixtures";

test.describe("Authenticated app shell", () => {
  test("shows core dashboard navigation and preserves project context", async ({
    api,
    page,
  }) => {
    await page.goto("/app/dashboard", { waitUntil: "domcontentloaded" });

    await expect(page.getByText("Project", { exact: true })).toBeVisible();
    for (const item of [
      "Dashboard",
      "Jobs",
      "Workflows",
      "Runs",
      "Schedules",
      "Dead Letter",
      "Logs",
      "Events",
      "Webhooks",
    ]) {
      await expect(
        page.locator('[data-sidebar="menu-button"]', { hasText: item })
      ).toBeVisible();
    }

    await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/app\/jobs/);
    await expect(page.getByRole("region", { name: "Jobs" })).toBeVisible();
    expect(api.getProjectId()).toBeTruthy();
  });

  test("keeps core dashboard navigation usable on mobile", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/app/dashboard", { waitUntil: "domcontentloaded" });

    await expect(page.getByText("Total Runs (24h)")).toBeVisible();
    await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("region", { name: "Jobs" })).toBeVisible();
    await page.goto("/app/workflows", { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("region", { name: "Workflows" })).toBeVisible();
  });
});

test.describe("Unauthenticated app shell", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("redirects protected dashboard routes to login", async ({ page }) => {
    await page.goto("/app/dashboard", { waitUntil: "domcontentloaded" });

    await expect(page).toHaveURL(/\/login/);
    await expect(
      page.getByRole("heading", { name: /sign in|log in/i })
    ).toBeVisible();
  });
});
