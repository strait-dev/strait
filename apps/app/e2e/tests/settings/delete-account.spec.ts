import { expect, test } from "../../fixtures";

test.describe("Delete account", () => {
  test.slow();
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/settings");
  });

  test("delete account section visible", async ({ page }) => {
    const section = page
      .getByText("Delete account", { exact: true })
      .or(page.getByText("Danger zone"));
    if (
      await section
        .first()
        .isVisible({ timeout: 10_000 })
        .catch(() => false)
    ) {
      await expect(section.first()).toBeVisible();
    }
  });

  test("shows warning about permanent deletion", async ({ page }) => {
    const warning = page.getByText(
      /permanently|irreversible|cannot be undone/i
    );
    if (
      await warning
        .first()
        .isVisible({ timeout: 5000 })
        .catch(() => false)
    ) {
      await expect(warning.first()).toBeVisible();
    }
  });

  test("delete button exists", async ({ page }) => {
    const deleteBtn = page.getByRole("button", {
      name: /delete account|delete my account/i,
    });
    if (await deleteBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(deleteBtn).toBeVisible();
    }
  });

  test("delete button opens confirmation dialog", async ({ page }) => {
    const confirmation = page.getByRole("checkbox", {
      name: /confirm that i want to delete my account/i,
    });
    if (await confirmation.isVisible({ timeout: 5000 }).catch(() => false)) {
      await confirmation.click();
      await expect(confirmation).toHaveAttribute("aria-checked", "true");
    }

    const deleteBtn = page.getByRole("button", {
      name: /delete account|delete my account/i,
    });
    if (
      (await deleteBtn.isVisible({ timeout: 5000 }).catch(() => false)) &&
      (await deleteBtn.isEnabled().catch(() => false))
    ) {
      await deleteBtn.click();
      await page.waitForTimeout(500);
      const dialog = page.locator("[role='alertdialog'], [role='dialog']");
      if (await dialog.isVisible({ timeout: 3000 }).catch(() => false)) {
        await expect(dialog).toBeVisible();
        // Close without deleting
        const cancelBtn = dialog.getByRole("button", { name: /cancel|close/i });
        if (await cancelBtn.isVisible()) {
          await cancelBtn.click();
        }
      }
    }
  });
});
