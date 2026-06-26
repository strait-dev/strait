import { resolve } from "node:path";
import { ApiHelper, expect, test } from "../../fixtures";
import {
  cleanupIsolatedOrgProject,
  createIsolatedOrgProject,
  type IsolatedOrgProject,
} from "../../support/auth-db";
import { TestDataFactory } from "../../support/test-data";

test.describe("Dogfood limited-user permissions and project isolation", () => {
  test.describe.configure({ timeout: 180_000 });
  test.use({
    storageState: resolve(process.cwd(), "playwright/.auth/limited-user.json"),
  });

  let api: ApiHelper;
  let data: TestDataFactory;
  let isolatedApi: ApiHelper;
  let isolatedData: TestDataFactory;
  let isolated: IsolatedOrgProject | null = null;
  let readableJob: { id: string; name: string };
  let readableWorkflow: { id: string; name: string };
  let readableWebhook: { id: string; webhook_url: string };
  let readableApiKey: { id: string; name: string };
  let readableFailedRun: { id: string; status: string };
  let readableSchedule: { id: string; name: string; enabled: boolean };
  let readableDlqRun: { id: string };
  let isolatedJobName: string;

  test.beforeAll(async () => {
    test.setTimeout(120_000);

    api = new ApiHelper();
    data = new TestDataFactory(api);

    const job = await data.job("dogfood-limited-readable", {
      endpoint_url: api.fakeEndpoint("/success"),
      description: "Visible to limited dogfood member",
    });
    readableJob = { id: job.id, name: job.name };
    const workflow = await data.workflow("dogfood-limited-workflow", [job.id]);
    readableWorkflow = { id: workflow.id, name: workflow.name };
    const webhook = await data.webhook("dogfood-limited-webhook", [
      "run.completed",
    ]);
    readableWebhook = {
      id: webhook.id,
      webhook_url: api.fakeEndpoint("/echo?name=dogfood-limited-webhook"),
    };
    const apiKey = await api.createApiKey({
      expires_in_days: 30,
      name: data.name("dogfood-limited-api-key"),
      scopes: ["jobs:read"],
    });
    readableApiKey = { id: apiKey.id, name: apiKey.name };
    data.cleanup.add(() => api.revokeApiKey(apiKey.id));
    const failedRun = await data.failedJobRun("dogfood-limited-run");
    const failedRunState = await api.getRun(failedRun.run.id);
    readableFailedRun = {
      id: failedRun.run.id,
      status: failedRunState.status,
    };
    const schedule = await api.createSchedule({
      name: data.name("dogfood-limited-schedule"),
      endpoint_url: api.fakeEndpoint("/success"),
      cron: "*/20 * * * *",
      timeout_secs: 10,
    });
    readableSchedule = {
      id: schedule.id,
      name: schedule.name,
      enabled: true,
    };
    data.cleanup.add(() => api.deleteSchedule(schedule.id));
    const dlq = await data.deadLetterRun("dogfood-limited-dlq");
    readableDlqRun = { id: dlq.run.id };

    isolated = await createIsolatedOrgProject(api, "dogfood-isolated");
    isolatedApi = new ApiHelper();
    isolatedApi.setProjectId(isolated.projectId);
    isolatedData = new TestDataFactory(isolatedApi);
    const isolatedJob = await isolatedData.job("dogfood-isolated-hidden", {
      endpoint_url: api.fakeEndpoint("/success"),
    });
    isolatedJobName = isolatedJob.name;
  });

  test.afterAll(async () => {
    await isolatedData?.cleanup.run();
    await data?.cleanup.run();
    await cleanupIsolatedOrgProject(api, isolated);
  });

  test("limited member can view active-project jobs but not isolated-project jobs", async ({
    page,
  }) => {
    await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });

    await page.getByLabel("Search").fill(readableJob.name);
    await expect(page.getByText(readableJob.name).first()).toBeVisible({
      timeout: 15_000,
    });

    await page.getByLabel("Search").fill(isolatedJobName);
    await expect(page.getByText(isolatedJobName)).not.toBeVisible();
  });

  test("direct create route fails cleanly for a limited member", async ({
    page,
  }) => {
    const deniedName = `e2e-dogfood-denied-job-${Date.now()}`;

    await page.goto("/app/jobs?create=1", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/app\/jobs$/);
    await expect(
      page.getByRole("heading", { name: "Create job" })
    ).toBeHidden();
    await expect(page.getByRole("button", { name: "Create job" })).toBeHidden();
    await expect
      .poll(
        async () =>
          (await api.listJobs({ limit: 100 })).data.filter(
            (job) => job.name === deniedName
          ),
        { timeout: 10_000 }
      )
      .toHaveLength(0);
  });

  test("direct schedule create route fails cleanly for a limited member", async ({
    page,
  }) => {
    const deniedName = `e2e-dogfood-denied-schedule-${Date.now()}`;

    await page.goto("/app/schedules?create=1", {
      waitUntil: "domcontentloaded",
    });
    await expect(page).toHaveURL(/\/app\/schedules$/);
    await expect(
      page.getByRole("heading", { name: "Create schedule" })
    ).toBeHidden();
    await expect(
      page.getByRole("button", { name: "Create schedule" })
    ).toBeHidden();
    await expect
      .poll(
        async () =>
          (await api.listJobs({ limit: 100 })).data.filter(
            (job) => job.name === deniedName
          ),
        { timeout: 10_000 }
      )
      .toHaveLength(0);
  });

  test("direct workflow create route fails cleanly for a limited member", async ({
    page,
  }) => {
    const deniedName = `e2e-dogfood-denied-workflow-${Date.now()}`;

    await page.goto("/app/workflows?create=1", {
      waitUntil: "domcontentloaded",
    });
    await expect(page).toHaveURL(/\/app\/workflows$/);
    await expect(
      page.getByRole("heading", { name: "Create workflow" })
    ).toBeHidden();
    await expect(
      page.getByRole("button", { name: "Create workflow" })
    ).toBeHidden();
    await expect
      .poll(
        async () =>
          (await api.listWorkflows({ limit: 100 })).data.filter(
            (workflow) => workflow.name === deniedName
          ),
        { timeout: 10_000 }
      )
      .toHaveLength(0);
  });

  test("limited member cannot trigger or delete jobs", async ({ page }) => {
    const beforeRuns = await api.listRuns({
      job_id: readableJob.id,
      limit: 100,
    });

    await page.goto(`/app/jobs/${readableJob.id}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.getByRole("heading", { name: readableJob.name })
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Trigger" })).toBeHidden();
    await expect(page.getByRole("button", { name: "Pause" })).toBeHidden();
    await expect
      .poll(
        async () =>
          (await api.listRuns({ job_id: readableJob.id, limit: 100 })).data
            .length,
        { timeout: 10_000 }
      )
      .toBe(beforeRuns.data.length);

    await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Search").fill(readableJob.name);
    const row = page.getByRole("row", { name: new RegExp(readableJob.name) });
    await row.getByRole("button", { name: "Row actions" }).click();
    await expect(page.getByRole("menuitem", { name: "Trigger" })).toBeHidden();
    await expect(page.getByRole("menuitem", { name: "Edit" })).toBeHidden();
    await expect(page.getByRole("menuitem", { name: "Delete" })).toBeHidden();
    await expect
      .poll(async () => (await api.getJob(readableJob.id)).id, {
        timeout: 10_000,
      })
      .toBe(readableJob.id);
  });

  test("limited member can view runs but cannot retry them", async ({
    page,
  }) => {
    await page.goto(`/app/runs/${readableFailedRun.id}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.getByRole("heading", { name: readableFailedRun.id })
    ).toBeVisible();

    const retryButton = page.getByRole("button", { name: "Retry" });
    await expect(retryButton).toBeHidden();

    await page.goto("/app/runs", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Search").fill(readableFailedRun.id);
    const row = page.getByRole("row", {
      name: new RegExp(readableFailedRun.id),
    });
    await expect(row.getByRole("button", { name: "Retry" })).toBeHidden();
    await expect(row.getByRole("button", { name: "Cancel" })).toBeHidden();
    const rowActions = row.first().getByRole("button", { name: "Row actions" });
    if (await rowActions.isVisible({ timeout: 1000 }).catch(() => false)) {
      await rowActions.click();
      await expect(page.getByRole("menuitem", { name: "Retry" })).toBeHidden();
      await expect(page.getByRole("menuitem", { name: "Cancel" })).toBeHidden();
    }
    await expect
      .poll(async () => (await api.getRun(readableFailedRun.id)).status, {
        timeout: 10_000,
      })
      .toBe(readableFailedRun.status);
  });

  test("limited member can view schedules but cannot trigger, pause, or delete them", async ({
    page,
  }) => {
    const beforeRuns = await api.listRuns({
      job_id: readableSchedule.id,
      limit: 100,
    });

    await page.goto(`/app/schedules/${readableSchedule.id}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.getByRole("heading", { name: readableSchedule.name })
    ).toBeVisible();

    await expect(page.getByRole("button", { name: "Trigger" })).toBeHidden();
    await expect
      .poll(
        async () =>
          (await api.listRuns({ job_id: readableSchedule.id, limit: 100 })).data
            .length,
        { timeout: 10_000 }
      )
      .toBe(beforeRuns.data.length);

    await expect(page.getByRole("button", { name: "Pause" })).toBeHidden();
    await expect
      .poll(async () => (await api.getJob(readableSchedule.id)).enabled, {
        timeout: 10_000,
      })
      .toBe(readableSchedule.enabled);

    await page.goto("/app/schedules", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Search").fill(readableSchedule.name);
    const row = page.getByRole("row", {
      name: new RegExp(readableSchedule.name),
    });
    await row.getByRole("button", { name: "Row actions" }).click();
    await expect(page.getByRole("menuitem", { name: "Trigger" })).toBeHidden();
    await expect(page.getByRole("menuitem", { name: "Pause" })).toBeHidden();
    await expect(page.getByRole("menuitem", { name: "Edit" })).toBeHidden();
    await expect(page.getByRole("menuitem", { name: "Delete" })).toBeHidden();
    await expect
      .poll(async () => (await api.getJob(readableSchedule.id)).id, {
        timeout: 10_000,
      })
      .toBe(readableSchedule.id);
  });

  test("limited member can view DLQ but cannot retry or discard dead-letter runs", async ({
    page,
  }) => {
    await page.goto("/app/dlq", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Search").fill(readableDlqRun.id);
    const row = page.getByRole("row", {
      name: new RegExp(readableDlqRun.id),
    });
    await expect(row).toBeVisible({ timeout: 15_000 });

    await expect(row.getByRole("button", { name: "Retry" })).toBeHidden();
    await expect(row.getByRole("button", { name: "Discard" })).toBeHidden();
    await expect
      .poll(
        async () =>
          (await api.listDlqEntries({ limit: 100 })).data.some(
            (entry) => entry.id === readableDlqRun.id
          ),
        { timeout: 10_000 }
      )
      .toBe(true);

    await expect
      .poll(
        async () =>
          (await api.listDlqEntries({ limit: 100 })).data.some(
            (entry) => entry.id === readableDlqRun.id
          ),
        { timeout: 10_000 }
      )
      .toBe(true);
  });

  test("limited member can view workflows but cannot trigger or delete them", async ({
    page,
  }) => {
    const beforeRuns = await api.listWorkflowRuns(readableWorkflow.id, {
      limit: 100,
    });

    await page.goto(`/app/workflows/${readableWorkflow.id}`, {
      waitUntil: "domcontentloaded",
    });
    await expect(
      page.getByRole("heading", { name: readableWorkflow.name })
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Trigger" })).toBeHidden();
    await expect(page.getByRole("button", { name: "Pause" })).toBeHidden();
    await expect
      .poll(
        async () =>
          (await api.listWorkflowRuns(readableWorkflow.id, { limit: 100 })).data
            .length,
        { timeout: 10_000 }
      )
      .toBe(beforeRuns.data.length);

    await page.goto("/app/workflows", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Search").fill(readableWorkflow.name);
    const row = page.getByRole("row", {
      name: new RegExp(readableWorkflow.name),
    });
    await row.getByRole("button", { name: "Row actions" }).click();
    await expect(page.getByRole("menuitem", { name: "Trigger" })).toBeHidden();
    await expect(page.getByRole("menuitem", { name: "Pause" })).toBeHidden();
    await expect(page.getByRole("menuitem", { name: "Delete" })).toBeHidden();
    await expect
      .poll(async () => (await api.getWorkflow(readableWorkflow.id)).id, {
        timeout: 10_000,
      })
      .toBe(readableWorkflow.id);
  });

  test("limited member can view webhooks but cannot create or delete them", async ({
    page,
  }) => {
    const deniedName = `e2e-dogfood-denied-webhook-${Date.now()}`;
    const deniedUrl = api.fakeEndpoint(`/echo?name=${deniedName}`);

    await page.goto("/app/webhooks", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Search").fill("dogfood-limited-webhook");
    await expect(page.getByText(readableWebhook.webhook_url)).toBeVisible({
      timeout: 15_000,
    });
    await expect(
      page.getByRole("button", { name: "Create webhook" })
    ).toBeHidden();

    await page.goto("/app/webhooks/new", { waitUntil: "domcontentloaded" });
    await page.getByLabel("URL").fill(deniedUrl);
    await page.getByText("Run completed").click();
    await page.getByRole("button", { name: "Create webhook" }).last().click();
    await expect(
      page.getByText(/Failed to create webhook/i).first()
    ).toBeVisible({
      timeout: 15_000,
    });
    await expect
      .poll(
        async () =>
          (await api.listWebhooks({ limit: 100 })).data.filter(
            (webhook) => webhook.webhook_url === deniedUrl
          ),
        { timeout: 10_000 }
      )
      .toHaveLength(0);

    await page.goto("/app/webhooks", { waitUntil: "domcontentloaded" });
    await page.getByLabel("Search").fill("dogfood-limited-webhook");
    const row = page.getByRole("row", {
      name: /dogfood-limited-webhook/,
    });
    await expect(row.getByRole("button", { name: "Delete" })).toBeHidden();
    await expect
      .poll(
        async () =>
          (await api.listWebhooks({ limit: 100 })).data.filter(
            (webhook) => webhook.id === readableWebhook.id
          ).length,
        { timeout: 10_000 }
      )
      .toBeGreaterThan(0);
  });

  test("limited member can view API key metadata but cannot create or revoke keys", async ({
    page,
  }) => {
    const deniedKeyName = `e2e-dogfood-denied-api-key-${Date.now()}`;

    await page.goto(`/app/org/${api.getOrgId()}`, {
      waitUntil: "domcontentloaded",
    });
    await page.getByRole("tab", { name: "API keys" }).click();
    await expect(page.getByText("Manage API keys")).toBeVisible();
    await expect(
      page.getByRole("row", { name: new RegExp(readableApiKey.name) })
    ).toBeVisible({ timeout: 15_000 });

    await expect(page.getByRole("button", { name: "Create key" })).toBeHidden();
    await expect
      .poll(async () => (await api.listApiKeys({ limit: 100 })).data, {
        timeout: 10_000,
      })
      .not.toContainEqual(expect.objectContaining({ name: deniedKeyName }));

    await page.goto(`/app/org/${api.getOrgId()}`, {
      waitUntil: "domcontentloaded",
    });
    await page.getByRole("tab", { name: "API keys" }).click();
    const readableKeyRow = page.getByRole("row", {
      name: new RegExp(readableApiKey.name),
    });
    await expect(readableKeyRow).toBeVisible({ timeout: 15_000 });
    const revokeButton = readableKeyRow.getByRole("button", {
      name: "Revoke",
    });
    await expect(revokeButton).toBeHidden();

    await expect
      .poll(async () => (await api.listApiKeys({ limit: 100 })).data, {
        timeout: 10_000,
      })
      .toContainEqual(expect.objectContaining({ id: readableApiKey.id }));
  });

  test("limited member sees organization settings as read-only", async ({
    page,
  }) => {
    await page.goto(`/app/org/${api.getOrgId()}`, {
      waitUntil: "domcontentloaded",
    });

    await expect(page.getByText("Organization details")).toBeVisible({
      timeout: 15_000,
    });
    await expect(page.getByLabel("Name")).toBeDisabled();
    await expect(page.getByLabel("Slug")).toBeDisabled();
    await expect(page.getByLabel("Email")).toBeDisabled();
    await expect(page.getByLabel("Website")).toBeDisabled();
    await expect(page.getByLabel("Description")).toBeDisabled();
    await expect(
      page.getByRole("button", { name: "Save changes" })
    ).toBeHidden();
    await expect(
      page.getByRole("button", { name: "Delete organization" })
    ).toBeHidden();

    await page.getByRole("tab", { name: "Team" }).click();
    await expect(page.getByText("Team members")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Invite Member" })
    ).toBeHidden();
    await expect(page.getByRole("button", { name: "Remove" })).toBeHidden();
  });
});
