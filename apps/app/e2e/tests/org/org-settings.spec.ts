import { expect, test } from "../../fixtures";

test.describe("Organization settings Navigation", () => {
  test("direct organization settings URL opens the tabbed settings shell", async ({
    api,
    page,
  }) => {
    await page.goto(`/app/org/${api.getOrgId()}`, {
      waitUntil: "domcontentloaded",
    });

    await expect(page).toHaveURL(new RegExp(`/app/org/${api.getOrgId()}`));
    await expect(
      page.getByRole("tab", { name: "Organization" })
    ).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("tab", { name: "Team" })).toBeVisible();
  });
});
