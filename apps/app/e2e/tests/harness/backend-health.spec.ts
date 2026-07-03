import { expect, test } from "../../fixtures";

test.describe("Backend-backed e2e harness", () => {
  test("connects to the Go API and the local fake job endpoint", async ({
    api,
  }) => {
    await expect(async () => {
      await api.health();
    }).toPass({ timeout: 10_000 });

    const fakeHealth = await fetch(`${api.getFakeEndpointUrl()}/health`);
    expect(fakeHealth.ok).toBe(true);

    const fakeJob = await fetch(api.fakeEndpoint("/success"), {
      method: "POST",
      body: JSON.stringify({ source: "playwright" }),
    });
    expect(fakeJob.ok).toBe(true);
    await expect(fakeJob.json()).resolves.toMatchObject({
      ok: true,
      body: { source: "playwright" },
    });
  });

  test("loads the authenticated dashboard shell with project context", async ({
    api,
    page,
  }) => {
    expect(api.getProjectId()).toBeTruthy();
    expect(api.getOrgId()).toBeTruthy();

    await page.goto("/app/dashboard", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/app\/dashboard/);
    await expect(page.getByText("Total runs (24h)")).toBeVisible();
  });
});
