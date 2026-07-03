import fs from "node:fs";
import { resolve } from "node:path";
import { expect, type Locator, type Page, test } from "@playwright/test";

const outboxPath = resolve("playwright/.auth/local-emails.jsonl");

test.describe("Magic link", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("page loads with email input", async ({ page }) => {
    await page.goto("/magic-link");
    await expect(page.getByPlaceholder("you@example.com")).toBeVisible();
  });

  test("back to sign in link works", async ({ page }) => {
    await page.goto("/magic-link");
    await page.getByRole("link", { name: /back to sign in/i }).click();
    await expect(page).toHaveURL(/login/);
  });

  test("page has correct heading", async ({ page }) => {
    await page.goto("/magic-link");
    await expect(page.getByText("Magic link sign in")).toBeVisible();
  });

  test("sends a magic link email and signs in from the link", async ({
    page,
  }) => {
    const email = process.env.E2E_USER_EMAIL;
    if (!email) {
      throw new Error("E2E_USER_EMAIL is required");
    }

    fs.rmSync(outboxPath, { force: true });

    await page.goto("/magic-link");
    await waitForClientRouter(page);
    const submit = page.getByRole("button", { name: "Send magic link" });
    await fillControlledInput(page.locator("#email"), email);
    await expect(submit).toBeEnabled();

    await submit.click();
    await expect(page.getByText("Check your email")).toBeVisible({
      timeout: 15_000,
    });

    const magicLink = await waitForMagicLink(email);
    await page.goto(magicLink, { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/app/, { timeout: 30_000 });
  });
});

async function waitForMagicLink(email: string) {
  const deadline = Date.now() + 15_000;

  while (Date.now() < deadline) {
    const link = readLatestMagicLink(email);
    if (link) {
      return link;
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }

  throw new Error(`No magic link email captured for ${email}`);
}

function readLatestMagicLink(email: string) {
  if (!fs.existsSync(outboxPath)) {
    return null;
  }

  const emails = fs
    .readFileSync(outboxPath, "utf-8")
    .split("\n")
    .filter(Boolean)
    .map(
      (line) => JSON.parse(line) as { to: string | string[]; html?: string }
    );

  for (const message of emails.reverse()) {
    const recipients = Array.isArray(message.to) ? message.to : [message.to];
    if (!recipients.includes(email)) {
      continue;
    }

    const urls = message.html?.match(/https?:\/\/[^"'<>\\\s]+/g) ?? [];
    const magicLink = urls.find(
      (url) => url.includes("magic-link") || url.includes("callbackURL")
    );
    if (magicLink) {
      return magicLink.replaceAll("&amp;", "&");
    }
  }

  return null;
}

async function fillControlledInput(locator: Locator, value: string) {
  await locator.click();
  await locator.pressSequentially(value);
}

async function waitForClientRouter(page: Page) {
  await page.waitForFunction(
    () => Boolean((globalThis as { __TSR_ROUTER__?: unknown }).__TSR_ROUTER__),
    null,
    { timeout: 10_000 }
  );
}
