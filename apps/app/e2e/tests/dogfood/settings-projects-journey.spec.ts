import type { Locator, Page } from "@playwright/test";
import { ApiHelper, expect, test } from "../../fixtures";
import {
  cleanupOrgProject,
  findOrgProjectByName,
  type OrgProject,
  setE2EUserActiveProjectId,
} from "../../support/auth-db";
import { selectTab } from "../../support/navigation";
import { TestDataFactory } from "../../support/test-data";

test.describe("Dogfood settings and project journeys", () => {
  test.describe.configure({ timeout: 180_000 });

  test("creates and revokes a project API key from organization settings", async ({
    api,
    page,
  }) => {
    const keyName = `e2e-dogfood-api-key-${Date.now()}`;

    await page.goto(`/app/org/${api.getOrgId()}`, {
      waitUntil: "domcontentloaded",
    });
    await selectTab(page, "API Keys");
    await expect(page.getByText("Manage API keys")).toBeVisible();

    await page.getByRole("button", { name: "Create key" }).click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByRole("heading", { name: "Create API key" })
    ).toBeVisible();
    await dialog.getByLabel("Key name").fill(keyName);
    await dialog.getByRole("combobox").click();
    await page.getByRole("option", { name: "30 days" }).click();
    await dialog.getByRole("button", { name: "Create key" }).click();

    await expect(
      dialog.getByRole("heading", { name: "API key created" })
    ).toBeVisible({ timeout: 15_000 });
    await expect(dialog.getByLabel("Created API key")).toHaveValue(/^strait_/);
    await expect
      .poll(async () => (await api.listApiKeys({ limit: 100 })).data)
      .toContainEqual(expect.objectContaining({ name: keyName }));
    await dialog.getByRole("button", { name: "Done" }).click();

    const row = page.getByRole("row", { name: new RegExp(keyName) });
    await expect(row).toBeVisible({ timeout: 15_000 });
    await row.getByRole("button", { name: "Revoke" }).click();
    await page.getByRole("button", { name: /Revoke key/i }).click();

    await expect
      .poll(async () => (await api.listApiKeys({ limit: 100 })).data)
      .not.toContainEqual(expect.objectContaining({ name: keyName }));
  });

  test("switches projects from the sidebar without leaking project data", async ({
    api,
    page,
  }) => {
    const defaultProjectId = api.getProjectId();
    const defaultData = new TestDataFactory(api);
    const secondProjectName = `dogfood-project-${Date.now()}`;
    let secondProject: OrgProject | null = null;
    let secondData: TestDataFactory | null = null;

    await setE2EUserActiveProjectId(defaultProjectId);

    try {
      const defaultJob = await defaultData.job("dogfood-default-project", {
        endpoint_url: api.fakeEndpoint("/success"),
      });

      await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });
      await expect(page.getByRole("region", { name: "Jobs" })).toBeVisible();
      await page.getByLabel("Search").fill(defaultJob.name);
      await expect(page.getByText(defaultJob.name)).toBeVisible({
        timeout: 15_000,
      });

      await createProjectFromSwitcher(
        page,
        "Default project",
        secondProjectName
      );
      await expect(page.getByText("Project created successfully!")).toBeVisible(
        {
          timeout: 15_000,
        }
      );
      await expect(
        page.getByRole("button", { name: secondProjectName }).first()
      ).toBeVisible({ timeout: 15_000 });
      await expect
        .poll(
          async () =>
            (await findOrgProjectByName(secondProjectName))?.projectId ?? null,
          { timeout: 15_000 }
        )
        .not.toBeNull();
      secondProject = await findOrgProjectByName(secondProjectName);
      if (!secondProject) {
        throw new Error(`Created project ${secondProjectName} was not found`);
      }

      const secondApi = new ApiHelper();
      secondApi.setProjectId(secondProject.projectId);
      secondData = new TestDataFactory(secondApi);
      const secondJob = await secondData.job("dogfood-switched-project", {
        endpoint_url: api.fakeEndpoint("/success"),
      });

      await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });
      await page.getByLabel("Search").fill(secondJob.name);
      await expect(page.getByText(secondJob.name)).toBeVisible({
        timeout: 15_000,
      });
      await page.getByLabel("Search").fill(defaultJob.name);
      await expect(page.getByText(defaultJob.name)).not.toBeVisible();

      await switchProject(page, secondProject.projectName, "Default project");
      await expect(
        page.getByRole("button", { name: "Default project" }).first()
      ).toBeVisible({ timeout: 15_000 });

      await page.goto("/app/jobs", { waitUntil: "domcontentloaded" });
      await page.getByLabel("Search").fill(defaultJob.name);
      await expect(page.getByText(defaultJob.name)).toBeVisible({
        timeout: 15_000,
      });
      await page.getByLabel("Search").fill(secondJob.name);
      await expect(page.getByText(secondJob.name)).not.toBeVisible();
    } finally {
      await setE2EUserActiveProjectId(defaultProjectId).catch(() => undefined);
      await secondData?.cleanup.run();
      await defaultData.cleanup.run();
      await cleanupOrgProject(api, secondProject);
    }
  });
});

async function createProjectFromSwitcher(
  page: Page,
  currentName: string,
  projectName: string
) {
  const newProjectItem = page.getByRole("menuitem", { name: "New project" });
  await openProjectMenu(page, currentName, newProjectItem);
  await newProjectItem.click();

  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("heading", { name: "Create new project" })
  ).toBeVisible({ timeout: 15_000 });
  await dialog.getByLabel("Name").fill(projectName);
  await dialog.getByRole("button", { name: "Create project" }).click();
}

async function switchProject(
  page: Page,
  currentName: string,
  targetName: string
) {
  const option = page.getByRole("menuitemcheckbox", { name: targetName });
  await openProjectMenu(page, currentName, option);
  await option.click();
}

async function openProjectMenu(
  page: Page,
  currentName: string,
  expectedItem: Locator
) {
  const trigger = page.getByRole("button", { name: currentName }).first();

  await expect(trigger).toBeVisible({ timeout: 15_000 });
  await expect(trigger).toBeEnabled({ timeout: 15_000 });
  for (let attempt = 0; attempt < 3; attempt += 1) {
    await trigger.click();
    if (await expectedItem.isVisible({ timeout: 2000 }).catch(() => false)) {
      break;
    }
  }
  await expect(expectedItem).toBeVisible({ timeout: 15_000 });
}
