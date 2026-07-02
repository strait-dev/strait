import { ApiHelper, expect, test } from "../../fixtures";
import { selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

test.describe("Clipboard interactions", () => {
  test("copies a newly created API key from the organization settings dialog", async ({
    page,
  }) => {
    const api = new ApiHelper();
    const data = new TestDataFactory(api);
    const keyName = data.name("clipboard-key");

    await page
      .context()
      .grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.goto(`/app/org/${api.getOrgId()}`, {
      waitUntil: "domcontentloaded",
    });
    await selectTab(page, "API Keys");
    await expect(page.getByText("Manage API keys")).toBeVisible();
    await page.getByRole("button", { name: "Create key" }).click();

    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Key Name").fill(keyName);
    await dialog.getByRole("combobox").click();
    await page.getByRole("option", { name: "30 days" }).click();
    await dialog.getByRole("button", { name: "Create key" }).click();
    await expect(
      dialog.getByRole("heading", { name: "API key created" })
    ).toBeVisible({ timeout: 15_000 });

    await expect(
      dialog.getByRole("button", { name: "Copy to Clipboard" })
    ).toBeEnabled();
    await dialog.getByRole("button", { name: "Copy to Clipboard" }).click();
    await dialog.getByRole("button", { name: "Done" }).click();
  });
});
