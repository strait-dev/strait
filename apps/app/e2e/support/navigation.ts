import type { Locator, Page } from "@playwright/test";
import { expect } from "../fixtures";

export const appRouteLoad = {
  timeout: 90_000,
  waitUntil: "domcontentloaded",
} as const;

/** Navigate to an app route and retry if the local dev loader drops. */
export async function gotoAndExpect(
  page: Page,
  path: string,
  locator: Locator
) {
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    await page.goto(path, appRouteLoad);
    try {
      await expect(locator).toBeVisible();
      return;
    } catch (error) {
      const retryButton = page.getByRole("button", { name: "Try again" });
      if (await retryButton.isVisible().catch(() => false)) {
        await retryButton.click();
        await expect(locator)
          .toBeVisible()
          .catch(() => undefined);
        if (await locator.isVisible().catch(() => false)) {
          return;
        }
      }
      if (attempt === 3) {
        throw error;
      }
    }
  }
}

/** Click a Base UI tab after route hydration and verify the selected state. */
export async function selectTab(page: Page, name: string) {
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    const tab = await tabLocator(page, name);
    await expect(tab).toBeVisible();

    try {
      await tab.click({ force: attempt > 1, timeout: 3000 });
      await expect(tab).toHaveAttribute("aria-selected", "true", {
        timeout: 3000,
      });
      return;
    } catch (error) {
      if (attempt === 3) {
        throw error;
      }
      await page.waitForTimeout(500);
    }
  }
}

async function tabLocator(page: Page, name: string) {
  const byRole = page.getByRole("tab", { name });
  if ((await byRole.count()) > 0) {
    return byRole;
  }

  return page
    .locator('[data-slot="tabs-trigger"]')
    .filter({ hasText: new RegExp(`^${escapeRegExp(name)}$`) })
    .first();
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
