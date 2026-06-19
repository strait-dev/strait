import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

test.describe("Dogfood navigation, search, and filters journey", () => {
  test.describe.configure({ timeout: 240_000 });

  test("navigates core surfaces from the sidebar and searches seeded records", async ({
    api,
    page,
  }) => {
    const data = new TestDataFactory(api);
    const job = await data.job("dogfood-nav-job", {
      endpoint_url: api.fakeEndpoint("/success"),
    });
    const completedRun = await api.triggerJob(job.id, {
      source: "dogfood-navigation",
    });
    await api.waitForRunStatus(completedRun.id, ["completed"], 60_000);

    const schedule = await api.createSchedule({
      name: data.name("dogfood-nav-schedule"),
      endpoint_url: api.fakeEndpoint("/success"),
      cron: "*/20 * * * *",
      timeout_secs: 10,
    });
    data.cleanup.add(() => api.deleteSchedule(schedule.id));

    const workflow = await data.workflow("dogfood-nav-workflow", [job.id]);
    const webhook = await data.webhook("dogfood-nav-webhook", [
      "run.completed",
    ]);
    const webhookUrl = api.fakeEndpoint("/echo?name=dogfood-nav-webhook");
    const dlq = await data.deadLetterRun("dogfood-nav-dlq");

    const eventKey = data.name("dogfood-nav-event");
    const eventWorkflow = await api.createWorkflow({
      name: data.name("dogfood-nav-event-workflow"),
      steps: [
        {
          step_ref: "await-navigation-event",
          step_type: "wait_for_event",
          event_key: eventKey,
          event_timeout_secs: 300,
        },
      ],
    });
    data.cleanup.add(() => api.deleteWorkflow(eventWorkflow.id));
    const workflowRun = await api.triggerWorkflow(eventWorkflow.id, {
      source: "dogfood-navigation",
      event_key: eventKey,
    });
    data.cleanup.add(() =>
      api.sendEvent(eventKey, { cleanup: true }).catch(() => undefined)
    );
    await api.waitForEventTrigger(
      (event) =>
        event.event_key === eventKey &&
        event.workflow_run_id === workflowRun.id &&
        event.status === "waiting",
      60_000
    );

    try {
      await page.goto("/app/dashboard", { waitUntil: "domcontentloaded" });
      await expect(page.getByText("Total Runs (24h)")).toBeVisible();
      await expect(page.getByText("Queued")).toBeVisible();

      await navigateViaSidebar(page, "Jobs", /\/app\/jobs/);
      await page.getByLabel("Search").fill(job.name);
      await expect(page.getByText(job.name)).toBeVisible({ timeout: 15_000 });

      await navigateViaSidebar(page, "Runs", /\/app\/runs/);
      await page.getByLabel("Search").fill(completedRun.id.slice(0, 8));
      await expect(
        page.getByRole("link", { name: completedRun.id.slice(0, 8) }).first()
      ).toBeVisible({ timeout: 15_000 });
      await expect(
        page.getByText(/completed|succeeded/i).first()
      ).toBeVisible();

      await navigateViaSidebar(page, "Schedules", /\/app\/schedules/);
      await page.getByLabel("Search").fill(schedule.name);
      await expect(page.getByText(schedule.name)).toBeVisible({
        timeout: 15_000,
      });

      await navigateViaSidebar(page, "Workflows", /\/app\/workflows/);
      await page.getByLabel("Search").fill(workflow.name);
      await expect(page.getByText(workflow.name)).toBeVisible({
        timeout: 15_000,
      });

      await navigateViaSidebar(page, "Dead Letter", /\/app\/dlq/);
      await page.getByLabel("Search").fill(dlq.run.id.slice(0, 8));
      await expect(
        page.getByRole("row", { name: new RegExp(dlq.run.id.slice(0, 8)) })
      ).toBeVisible({
        timeout: 15_000,
      });

      await navigateViaSidebar(page, "Events", /\/app\/events/);
      await page.getByLabel("Search").fill(eventKey);
      await expect(
        page.getByRole("row", { name: new RegExp(`Waiting ${eventKey}`) })
      ).toBeVisible({ timeout: 15_000 });

      await navigateViaSidebar(page, "Logs", /\/app\/logs/);
      await page.getByLabel("Search").fill(eventKey);
      await expect(page.getByText(eventKey)).toBeVisible({ timeout: 15_000 });

      await navigateViaSidebar(page, "Webhooks", /\/app\/webhooks/);
      await page.getByLabel("Search").fill("dogfood-nav-webhook");
      expect(webhook.id).toBeTruthy();
      await expect(page.getByText(webhookUrl)).toBeVisible({
        timeout: 15_000,
      });
    } finally {
      await data.cleanup.run();
    }
  });

  test("uses cursor pagination and search together on a resource list", async ({
    api,
    page,
  }) => {
    const data = new TestDataFactory(api);
    const prefix = data.name("dogfood-pagination-job");

    try {
      for (let index = 0; index < 12; index += 1) {
        await data.job(`${prefix}-${String(index).padStart(2, "0")}`, {
          endpoint_url: api.fakeEndpoint("/success"),
        });
      }

      await page.goto("/app/jobs?perPage=10", {
        waitUntil: "domcontentloaded",
      });

      await expect(page.getByText("10 rows")).toBeVisible({
        timeout: 15_000,
      });
      await expect(
        page.getByRole("button", { name: "Go to next page" })
      ).toBeEnabled();
      await page.getByRole("button", { name: "Go to next page" }).click();
      await expect(page).toHaveURL(/cursor=/);
      await expect(
        page.getByRole("button", { name: "Go to previous page" })
      ).toBeEnabled();

      await page.getByRole("button", { name: "Go to previous page" }).click();
      await expect(page).not.toHaveURL(/cursor=/);

      await page.getByLabel("Search").fill(prefix);
      await expect(page.getByText(prefix).first()).toBeVisible({
        timeout: 15_000,
      });
      await expect(page.getByRole("region", { name: "Jobs" })).toBeVisible();
    } finally {
      await data.cleanup.run();
    }
  });
});

async function navigateViaSidebar(page: Page, label: string, url: RegExp) {
  await page
    .locator('[data-sidebar="menu-button"]', { hasText: label })
    .click();
  await expect(page).toHaveURL(url);
}
