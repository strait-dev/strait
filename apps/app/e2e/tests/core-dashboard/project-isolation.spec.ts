import { ApiHelper, expect, test } from "../../fixtures";
import {
  cleanupIsolatedOrgProject,
  createIsolatedOrgProject,
  type IsolatedOrgProject,
} from "../../support/auth-db";
import { TestDataFactory } from "../../support/test-data";

let activeApi: ApiHelper;
let activeData: TestDataFactory;
let isolatedApi: ApiHelper;
let isolatedData: TestDataFactory;
let isolated: IsolatedOrgProject | null = null;
let activeJobName: string;
let isolatedJobName: string;

test.describe("Project isolation", () => {
  test.beforeAll(async () => {
    activeApi = new ApiHelper();
    activeData = new TestDataFactory(activeApi);

    isolated = await createIsolatedOrgProject(activeApi, "e2e-isolated");
    isolatedApi = new ApiHelper();
    isolatedApi.setProjectId(isolated.projectId);
    isolatedData = new TestDataFactory(isolatedApi);

    const activeJob = await activeData.job("active-scope");
    const isolatedJob = await isolatedData.job("isolated-scope");
    activeJobName = activeJob.name;
    isolatedJobName = isolatedJob.name;
  });

  test.afterAll(async () => {
    await isolatedData?.cleanup.run();
    await activeData?.cleanup.run();
    await cleanupIsolatedOrgProject(activeApi, isolated);
  });

  test("scopes backend lists by active project id", async () => {
    const activeJobs = await activeApi.listJobs({ search: activeJobName });
    expect(activeJobs.data.map((job) => job.name)).toContain(activeJobName);

    const crossProjectLeak = await activeApi.listJobs({
      search: isolatedJobName,
    });
    expect(crossProjectLeak.data.map((job) => job.name)).not.toContain(
      isolatedJobName
    );

    const isolatedJobs = await isolatedApi.listJobs({
      search: isolatedJobName,
    });
    expect(isolatedJobs.data.map((job) => job.name)).toContain(isolatedJobName);
  });

  test("shows only active project jobs in the dashboard UI", async ({
    page,
  }) => {
    await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("region", { name: "Jobs" })).toBeVisible();
    await page.getByLabel("Search").fill(activeJobName);
    await expect(page.getByText(activeJobName)).toBeVisible();

    await page.getByLabel("Search").fill(isolatedJobName);
    await expect(page.getByText(isolatedJobName)).not.toBeVisible();
  });
});
