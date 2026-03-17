import { buildEntityRoutes } from "./entity-routes";

/**
 * `strait workflows` operational command group.
 */
export const workflowsRoutes = buildEntityRoutes({
  groupName: "workflows",
  groupBrief: "Workflow management commands",
  basePath: "/v1/workflows",
  idPlaceholder: "workflowID",
  idCandidates: ["id", "workflow_id", "slug", "name"],
  listBrief: "List workflows",
  getBrief: "Get one workflow",
  supportProjectFilter: true,
});
