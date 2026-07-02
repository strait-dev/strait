import { ApiHelper, expect, test } from "../../fixtures";
import { setE2EUserOrganizationRole } from "../../support/auth-db";
import { gotoAndExpect, selectTab } from "../../support/navigation";

test.describe("Organization RBAC permissions", () => {
  test.describe.configure({ timeout: 120_000 });

  test("hides team management actions from organization members", async ({
    page,
  }) => {
    const api = new ApiHelper();
    const orgId = api.getOrgId();

    await setE2EUserOrganizationRole(orgId, "member");
    try {
      await gotoAndExpect(
        page,
        `/app/org/${orgId}`,
        page.getByRole("tab", { name: "Team" })
      );
      await selectTab(page, "Team");

      await expect(page.getByText("Manage who has access")).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Invite member" })
      ).toHaveCount(0);
      await expect(page.getByRole("button", { name: "Leave" })).toBeVisible();
    } finally {
      await setE2EUserOrganizationRole(orgId, "owner");
    }
  });

  test("restores owner-only team management controls after role elevation", async ({
    page,
  }) => {
    const api = new ApiHelper();
    const orgId = api.getOrgId();

    await setE2EUserOrganizationRole(orgId, "owner");
    await gotoAndExpect(
      page,
      `/app/org/${orgId}`,
      page.getByRole("tab", { name: "Team" })
    );
    await selectTab(page, "Team");

    await expect(
      page.getByRole("button", { name: "Invite member" })
    ).toBeVisible();
  });
});
