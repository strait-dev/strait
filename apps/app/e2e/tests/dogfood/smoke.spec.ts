import { expect, test } from "../../fixtures";

test.describe("Dogfood local stack smoke", () => {
  test("verifies the reusable local stack is ready for browser journeys", async ({
    api,
    page,
  }) => {
    await expect(async () => {
      await api.health();
    }).toPass({ timeout: 15_000 });

    const fakeEndpointHealth = await fetch(
      `${api.getFakeEndpointUrl()}/health`
    );
    expect(fakeEndpointHealth.ok).toBe(true);

    await page.goto("/app/dashboard", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/app\/dashboard/);
    await expect(page.getByText("Total runs (24h)")).toBeVisible();
    await expect(page.getByText("Queued")).toBeVisible();

    const stats = await api.getStats();
    expect(typeof stats.queued).toBe("number");
    expect(typeof stats.executing).toBe("number");
    expect(typeof stats.delayed).toBe("number");

    const analytics = await api.getAnalytics();
    expect(analytics.throughput).toBeDefined();
    expect(analytics.health_summary).toBeDefined();
    expect(Array.isArray(analytics.slowest_jobs)).toBe(true);
  });
});
