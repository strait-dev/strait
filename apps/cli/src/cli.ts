import { buildApplication, buildRouteMap, run } from "@stricli/core";
import { apiKeysRoutes } from "./commands/api-keys";
import { authRoutes, authWhoamiCommand } from "./commands/auth";
import { buildCommandRoute } from "./commands/build";
import { contextRoutes } from "./commands/context";
import { deployCommandRoute } from "./commands/deploy";
import { devCommandRoute } from "./commands/dev";
import { diffCommandRoute } from "./commands/diff";
import { doctorCommand } from "./commands/doctor";
import { eventsRoutes } from "./commands/events";
import { healthCommand } from "./commands/health";
import { initCommand } from "./commands/init";
import { jobsRoutes } from "./commands/jobs";
import { promoteCommandRoute } from "./commands/promote";
import { rollbackCommandRoute } from "./commands/rollback";
import { runsRoutes } from "./commands/runs";
import { secretsRoutes } from "./commands/secrets";
import { statsCommand } from "./commands/stats";
import { workflowRunsRoutes } from "./commands/workflow-runs";
import { workflowsRoutes } from "./commands/workflows";
import type { StraitCommandContext } from "./context";

const routes = buildRouteMap({
  routes: {
    "api-keys": apiKeysRoutes,
    auth: authRoutes,
    build: buildCommandRoute,
    context: contextRoutes,
    deploy: deployCommandRoute,
    diff: diffCommandRoute,
    doctor: doctorCommand,
    dev: devCommandRoute,
    events: eventsRoutes,
    health: healthCommand,
    init: initCommand,
    jobs: jobsRoutes,
    promote: promoteCommandRoute,
    rollback: rollbackCommandRoute,
    runs: runsRoutes,
    secrets: secretsRoutes,
    stats: statsCommand,
    workflows: workflowsRoutes,
    "workflow-runs": workflowRunsRoutes,
    whoami: authWhoamiCommand,
  },
  docs: {
    brief: "Unified Strait CLI",
  },
});

/**
 * Unified CLI application descriptor.
 */
export const app = buildApplication<StraitCommandContext>(routes, {
  name: "strait",
  versionInfo: {
    currentVersion: "0.0.0-dev",
  },
});

/**
 * Executes the CLI with the provided argv and command context.
 */
export const runCli = (
  argv: readonly string[],
  context: StraitCommandContext
): Promise<void> => run(app, [...argv], context);
