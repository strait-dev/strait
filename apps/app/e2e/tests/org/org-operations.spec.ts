import { expect, test } from "../../fixtures";

test.describe("Organization operations", () => {
  test.beforeEach(async ({ api, page }) => {
    await page.goto(`/app/org/${api.getOrgId()}`, {
      waitUntil: "domcontentloaded",
    });
  });

  test("renders the primary organization settings tabs", async ({ page }) => {
    await expect(page.getByRole("tab", { name: "Organization" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Subscription" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "API Keys" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Team" })).toBeVisible();
  });
});
