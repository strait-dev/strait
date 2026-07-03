import { randomUUID } from "node:crypto";
import { ApiHelper, expect, test } from "../../fixtures";
import {
  cleanupIsolatedOrgProject,
  createIsolatedOrgProject,
  removeE2EUserMembership,
  setE2EUserActiveProjectId,
} from "../../support/auth-db";
import { TestDataFactory } from "../../support/test-data";

test.describe("Core dashboard negative paths", () => {
  test.describe.configure({ timeout: 180_000 });

  let api: ApiHelper;
  let data: TestDataFactory;

  test.beforeEach(() => {
    api = new ApiHelper();
    data = new TestDataFactory(api);
  });

  test.afterEach(async () => {
    await data?.cleanup.run();
  });

  test("recovers when the active project id is stale", async ({ page }) => {
    const originalProjectId = api.getProjectId();
    await setE2EUserActiveProjectId(randomUUID());

    try {
      await page.goto("/app/dashboard", { waitUntil: "domcontentloaded" });

      await expect(page.locator("body")).toBeVisible();
      await expect(page.getByText("Overview", { exact: true })).toBeVisible();
      await expect(page.getByText("Total runs (24h)")).toBeVisible();
      await expect(
        page.getByText("Run activity", { exact: true })
      ).toBeVisible();
    } finally {
      await setE2EUserActiveProjectId(originalProjectId);
    }
  });

  test("recovers when the active project fails membership checks", async ({
    page,
  }) => {
    const originalProjectId = api.getProjectId();
    const isolated = await createIsolatedOrgProject(api, "e2e-permission");

    try {
      await removeE2EUserMembership(isolated.orgId);
      await setE2EUserActiveProjectId(isolated.projectId);

      await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });

      await expect(page.locator("body")).toBeVisible();
      await expect(
        page.getByText(/failed|forbidden|permission|access|project/i).first()
      ).toBeVisible();
    } finally {
      await setE2EUserActiveProjectId(originalProjectId);
      await cleanupIsolatedOrgProject(api, isolated);
    }
  });

  test("keeps disabled jobs visible and rejects manual triggers", async ({
    page,
  }) => {
    const job = await data.job("disabled-trigger", { enabled: false });

    const trigger = await api.requestRaw("POST", `/v1/jobs/${job.id}/trigger`, {
      project_id: api.getProjectId(),
      payload: { expected: "disabled" },
    });

    expect(trigger.status).toBe(400);
    expect(String(trigger.text).toLowerCase()).toContain("disabled");

    await page.goto(`/app/jobs/${job.id}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("heading", { name: job.name })).toBeVisible();
    await expect(page.getByText("Paused")).toBeVisible();
    await expect(page.getByRole("button", { name: "Resume" })).toBeVisible();
  });

  test("rejects invalid workflow definitions before they reach the dashboard", async () => {
    const invalidWorkflow = await api.requestRaw("POST", "/v1/workflows", {
      project_id: api.getProjectId(),
      name: `e2e-invalid-workflow-${randomUUID().slice(0, 8)}`,
      slug: `e2e-invalid-workflow-${randomUUID().slice(0, 8)}`,
      steps: [
        {
          step_ref: "missing-job",
          job_id: randomUUID(),
        },
      ],
    });

    expect([400, 404, 422]).toContain(invalidWorkflow.status);
  });

  test("recovers from transient backend 5xx responses on dashboard refresh", async ({
    page,
  }) => {
    let interceptedStats = false;
    await page.route("**/v1/stats", (route) => {
      if (interceptedStats) {
        return route.continue();
      }
      interceptedStats = true;
      return route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "e2e injected stats failure" }),
      });
    });

    await page.goto("/app/dashboard", { waitUntil: "domcontentloaded" });
    await expect(page.locator("body")).toBeVisible();
    await expect(page.getByText("Overview", { exact: true })).toBeVisible();
    await expect(page.getByText("Run activity", { exact: true })).toBeVisible();
  });
});
