import { expect, test } from "../../fixtures";

test.describe("Dashboard Charts", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/dashboard");
  });

  test("run activity chart renders", async ({ page }) => {
    await expect(page.getByText("Run activity", { exact: true })).toBeVisible();
  });

  test("status distribution chart renders", async ({ page }) => {
    await expect(
      page.getByText("Status distribution", { exact: true })
    ).toBeVisible();
  });

  test("failed runs by job chart renders", async ({ page }) => {
    await expect(
      page.getByText("Failed runs by Job", { exact: true })
    ).toBeVisible();
  });

  test("throughput chart renders", async ({ page }) => {
    await expect(page.getByText("Throughput (24h)")).toBeVisible();
  });

  test("dashboard charts render without Recharts zero-size warnings", async ({
    page,
  }) => {
    const rechartsWarnings: string[] = [];
    page.on("console", (message) => {
      const text = message.text();
      if (
        text.includes(
          "The width(0) and height(0) of chart should be greater"
        ) ||
        text.includes("The width(-1) and height(-1) of chart should be greater")
      ) {
        rechartsWarnings.push(text);
      }
    });

    await page.goto("/app/dashboard", { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Run activity", { exact: true })).toBeVisible();
    await expect(page.getByText("Status distribution")).toBeVisible();
    await expect(page.getByText("Throughput (24h)")).toBeVisible();

    expect(rechartsWarnings).toEqual([]);
  });

  test("billing charts render without Recharts zero-size warnings", async ({
    page,
  }) => {
    const rechartsWarnings: string[] = [];
    page.on("console", (message) => {
      const text = message.text();
      if (
        text.includes(
          "The width(0) and height(0) of chart should be greater"
        ) ||
        text.includes("The width(-1) and height(-1) of chart should be greater")
      ) {
        rechartsWarnings.push(text);
      }
    });

    await page.goto("/app/billing", { waitUntil: "domcontentloaded" });
    await expect(
      page.getByRole("heading", { exact: true, name: "Billing" })
    ).toBeVisible();
    await expect(page.locator("main")).toBeVisible();

    expect(rechartsWarnings).toEqual([]);
  });
});
