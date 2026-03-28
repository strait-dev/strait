import { spawn } from "node:child_process";
import { once } from "node:events";
import { ensureLocalDevUser } from "./lib/local-auth-user";
import {
  applyLocalDefaults,
  migrateAuthDatabase,
  resolveAvailableDevServerOptions,
  waitForBaseURL,
  withResolvedDevServerArgs,
} from "./lib/local-bootstrap";

async function main() {
  const args = process.argv.slice(2);
  const resolvedOptions = await resolveAvailableDevServerOptions(process.env, args);
  const devArgs = withResolvedDevServerArgs(args, resolvedOptions);
  const env = applyLocalDefaults(process.env, args, resolvedOptions);
  const cwd = new URL("..", import.meta.url);

  Object.assign(process.env, env);

  console.log("Bootstrapping local auth schema...");
  await migrateAuthDatabase();

  const child = spawn("bun", ["run", "dev:raw", ...devArgs], {
    cwd,
    env,
    stdio: "inherit",
  });

  const exitPromise = once(child, "exit").then(([code, signal]) => {
    if (signal) {
      process.kill(process.pid, signal);
      return;
    }
    process.exit(code ?? 0);
  });

  try {
    await waitForBaseURL(env.BETTER_AUTH_URL!);
    const seeded = await ensureLocalDevUser({
      authDbUrl: env.AUTH_DATABASE_URL!,
      baseURL: env.BETTER_AUTH_URL!,
      apiURL: env.STRAIT_API_URL!,
      internalSecret: env.INTERNAL_SECRET!,
      email: env.LOCAL_DEV_USER_EMAIL,
      password: env.LOCAL_DEV_USER_PASSWORD,
      name: env.LOCAL_DEV_USER_NAME,
    });

    console.log(
      [
        "",
        "Local app bootstrap complete.",
        `  App URL: ${env.BETTER_AUTH_URL}`,
        `  Local user: ${seeded.email}`,
        `  Password: ${seeded.password}`,
        `  Organization: ${seeded.orgId}`,
        `  Project: ${seeded.projectId}`,
        "",
      ].join("\n")
    );
  } catch (error) {
    child.kill("SIGTERM");
    throw error;
  }

  await exitPromise;
}

main().catch((error) => {
  console.error("Local dev bootstrap failed:", error);
  process.exit(1);
});
