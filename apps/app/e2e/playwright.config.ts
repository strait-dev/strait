import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig, devices } from "@playwright/test";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const isCI = !!process.env.CI;

export default defineConfig({
  testDir: "./tests",
  fullyParallel: true,
  forbidOnly: isCI,
  retries: isCI ? 1 : 0,
  workers: isCI ? 2 : undefined,
  maxFailures: isCI ? 20 : undefined,
  reporter: isCI ? [["html"], ["github"]] : "html",
  timeout: 15_000,
  expect: {
    timeout: 8000,
  },
  globalSetup: resolve(__dirname, "global-setup.ts"),
  globalTeardown: resolve(__dirname, "global-teardown.ts"),
  use: {
    baseURL: process.env.EXPECT_BASE_URL || "http://localhost:5173",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    storageState: resolve(__dirname, "../playwright/.auth/user.json"),
    navigationTimeout: 15_000,
    actionTimeout: 10_000,
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: isCI
    ? undefined
    : {
        command: "DISABLE_NGROK=1 infisical run --env=dev -- bun run dev",
        url: "http://localhost:5173",
        reuseExistingServer: true,
        timeout: 120_000,
      },
});
