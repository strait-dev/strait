import { ApiHelper, expect, test } from "../../fixtures";
import { gotoAndExpect, selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

// Unique key templates so each getByText(key) assertion is unambiguous and the
// jobs never collide on a shared singleton lock.
const STAMP = Date.now();
const DROP_KEY = `drop-key-${STAMP}`;
const REPLACE_KEY = `replace-key-${STAMP}`;
const DEPTH_KEY = `depth-key-${STAMP}`;
const UNBOUNDED_KEY = `unbounded-key-${STAMP}`;
const EMPTY_KEY = `empty-key-${STAMP}`;
const MAX_QUEUE_DEPTH = 5;

type Job = { id: string; name: string };

/** The read-only singleton config rows live on the default job detail view. */
function configRow(page: import("@playwright/test").Page, label: string) {
  return page
    .locator("div.items-center.justify-between")
    .filter({ hasText: label })
    .last();
}

test.describe("Singleton configuration display", () => {
  test.describe.configure({ timeout: 90_000 });

  let api: ApiHelper;
  let data: TestDataFactory;
  let dropJob: Job;
  let replaceJob: Job;
  let depthJob: Job;
  let unboundedJob: Job;
  let emptyJob: Job;
  let plainJob: Job;

  test.beforeAll(async () => {
    api = new ApiHelper();
    data = new TestDataFactory(api);

    // None of these are triggered, so no runs hold a lock — the config rows and
    // the empty holders table can be asserted without any in-flight work.
    [dropJob, replaceJob, depthJob, unboundedJob, emptyJob, plainJob] =
      await Promise.all([
        data.job("singleton-drop", {
          singleton_key_expr: { template: DROP_KEY },
          singleton_on_conflict: "drop",
        }),
        data.job("singleton-replace", {
          singleton_key_expr: { template: REPLACE_KEY },
          singleton_on_conflict: "replace",
        }),
        data.job("singleton-depth", {
          singleton_key_expr: { template: DEPTH_KEY },
          singleton_on_conflict: "queue",
          singleton_max_queue_depth: MAX_QUEUE_DEPTH,
        }),
        data.job("singleton-unbounded", {
          singleton_key_expr: { template: UNBOUNDED_KEY },
          singleton_on_conflict: "queue",
        }),
        data.job("singleton-empty", {
          singleton_key_expr: { template: EMPTY_KEY },
          singleton_on_conflict: "queue",
        }),
        data.job("plain"),
      ]);
  });

  test.afterAll(async () => {
    await data?.cleanup.run();
  });

  test("drop policy shows mode and key but no queue-depth row", async ({
    page,
  }) => {
    await gotoAndExpect(
      page,
      `/app/jobs/${dropJob.id}`,
      page.getByRole("heading", { name: dropJob.name })
    );

    await expect(page.getByText("Singleton Mode")).toBeVisible();
    await expect(configRow(page, "Singleton Mode")).toContainText("Drop");
    await expect(page.getByText("Singleton Key")).toBeVisible();
    await expect(page.getByText(DROP_KEY).first()).toBeVisible();
    // Max Queue Depth is only meaningful for the queue policy.
    await expect(page.getByText("Max Queue Depth")).toHaveCount(0);
  });

  test("replace policy shows the replace mode", async ({ page }) => {
    await gotoAndExpect(
      page,
      `/app/jobs/${replaceJob.id}`,
      page.getByRole("heading", { name: replaceJob.name })
    );

    await expect(configRow(page, "Singleton Mode")).toContainText("Replace");
    await expect(page.getByText(REPLACE_KEY).first()).toBeVisible();
    await expect(page.getByText("Max Queue Depth")).toHaveCount(0);
  });

  test("queue policy shows the configured max queue depth", async ({
    page,
  }) => {
    await gotoAndExpect(
      page,
      `/app/jobs/${depthJob.id}`,
      page.getByRole("heading", { name: depthJob.name })
    );

    await expect(configRow(page, "Singleton Mode")).toContainText("Queue");
    await expect(page.getByText("Max Queue Depth")).toBeVisible();
    await expect(configRow(page, "Max Queue Depth")).toContainText(
      String(MAX_QUEUE_DEPTH)
    );
  });

  test("queue policy without a depth shows Unbounded", async ({ page }) => {
    await gotoAndExpect(
      page,
      `/app/jobs/${unboundedJob.id}`,
      page.getByRole("heading", { name: unboundedJob.name })
    );

    await expect(page.getByText("Max Queue Depth")).toBeVisible();
    await expect(configRow(page, "Max Queue Depth")).toContainText("Unbounded");
  });

  test("a singleton job with no runs shows the empty holders table", async ({
    page,
  }) => {
    await gotoAndExpect(
      page,
      `/app/jobs/${emptyJob.id}`,
      page.getByRole("heading", { name: emptyJob.name })
    );

    await selectTab(page, "Singletons");

    await expect(
      page.getByRole("table", { name: "Singleton holders" })
    ).toBeVisible();
    await expect(page.getByText("No keys currently held")).toBeVisible();
  });

  test("a non-singleton job hides the singleton tab and config", async ({
    page,
  }) => {
    await gotoAndExpect(
      page,
      `/app/jobs/${plainJob.id}`,
      page.getByRole("heading", { name: plainJob.name })
    );

    await expect(page.getByRole("tab", { name: "Singletons" })).toHaveCount(0);
    await expect(page.getByText("Singleton Mode")).toHaveCount(0);
  });
});
