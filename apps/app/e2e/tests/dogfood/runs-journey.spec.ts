import { expect, test } from "../../fixtures";
import { TestDataFactory } from "../../support/test-data";

test.describe("Dogfood runs journey", () => {
  test.describe.configure({ timeout: 210_000 });

  test("finds completed and failed runs, opens detail, and retries from the runs list", async ({
    api,
    page,
  }) => {
    const data = new TestDataFactory(api);
    const completed = await data.successfulJobRun("dogfood-run-completed");
    const failed = await data.failedJobRun("dogfood-run-failed");

    try {
      await page.goto("/app/runs", { waitUntil: "domcontentloaded" });
      await expect(page.getByRole("region", { name: "Runs" })).toBeVisible();

      await page.getByLabel("Search").fill(completed.run.id);
      await expect(
        page.getByRole("link", { name: completed.run.id.slice(0, 8) }).first()
      ).toBeVisible({ timeout: 15_000 });
      await expect(
        page.getByText(/completed|succeeded/i).first()
      ).toBeVisible();

      await page.getByLabel("Search").fill(failed.run.id);
      const failedRow = page
        .getByRole("row", {
          name: new RegExp(failed.run.id.slice(0, 8)),
        })
        .first();
      await expect(failedRow).toBeVisible({ timeout: 15_000 });
      await expect(page.getByText(/failed|dead letter/i).first()).toBeVisible();

      await failedRow.getByRole("button", { name: "Retry" }).click();
      await expect
        .poll(async () => (await api.getRun(failed.run.id)).status, {
          timeout: 15_000,
        })
        .toMatch(
          /queued|dequeued|executing|waiting|completed|failed|dead_letter/
        );

      await page
        .getByRole("link", { name: failed.run.id.slice(0, 8) })
        .first()
        .click();
      await expect(page).toHaveURL(new RegExp(`/app/runs/${failed.run.id}`));
      await expect(
        page.getByRole("heading", { name: failed.run.id })
      ).toBeVisible({ timeout: 15_000 });
      await expect(page.getByText("What happened")).toBeVisible();
      await expect(page.getByRole("tab", { name: /Payload/ })).toBeVisible();
      await expect(page.getByRole("tab", { name: /Response/ })).toBeVisible();
    } finally {
      await data.cleanup.run();
    }
  });

  test("cancels an active run from the run detail page", async ({
    api,
    page,
  }) => {
    const data = new TestDataFactory(api);
    const job = await data.job("dogfood-run-cancel", {
      endpoint_url: api.fakeEndpoint("/timeout?delay_ms=60000"),
      max_attempts: 1,
      timeout_secs: 120,
    });
    const run = await api.triggerJob(job.id, { scenario: "dogfood-cancel" });
    data.cleanup.add(() => api.cancelRun(run.id).catch(() => undefined));

    try {
      await expect(async () => {
        const current = await api.getRun(run.id);
        expect(["queued", "dequeued", "executing", "waiting"]).toContain(
          current.status
        );
      }).toPass({ timeout: 30_000 });

      await page.goto(`/app/runs/${run.id}`, {
        waitUntil: "domcontentloaded",
      });
      await expect(page.getByRole("heading", { name: run.id })).toBeVisible();
      await page.getByRole("button", { name: "Cancel" }).click();
      await page.getByRole("button", { name: "Cancel run" }).click();

      await expect
        .poll(async () => (await api.getRun(run.id)).status, {
          timeout: 60_000,
        })
        .toBe("canceled");
    } finally {
      await data.cleanup.run();
    }
  });
});
