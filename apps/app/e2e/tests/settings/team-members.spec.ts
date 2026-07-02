import { ApiHelper, expect, test } from "../../fixtures";
import { selectTab } from "../../support/navigation";

test.describe("Organization team members", () => {
  test("shows the signed-in owner and opens the invite dialog", async ({
    page,
  }) => {
    const api = new ApiHelper();
    const email = process.env.E2E_USER_EMAIL;

    await page.goto(`/app/org/${api.getOrgId()}`, {
      waitUntil: "domcontentloaded",
    });
    await selectTab(page, "Team");

    await expect(page.getByText("Manage who has access")).toBeVisible();
    if (email) {
      await expect(page.getByText(email)).toBeVisible();
    }
    await expect(page.getByText(/owner/i).first()).toBeVisible();

    await page.getByRole("button", { name: "Invite member" }).click();
    await expect(
      page.getByRole("dialog").getByRole("heading", {
        name: "Invite a team member",
      })
    ).toBeVisible();
  });
});
