import { buildEntityRoutes } from "./entity-routes";

/**
 * `strait workflow-runs` operational command group.
 */
export const workflowRunsRoutes = buildEntityRoutes({
  groupName: "workflow-runs",
  groupBrief: "Workflow run management commands",
  basePath: "/v1/workflow-runs",
  idPlaceholder: "workflowRunID",
  idCandidates: ["id", "workflow_run_id", "status", "workflow_id"],
  listBrief: "List workflow runs",
  getBrief: "Get one workflow run",
  supportProjectFilter: true,
});
