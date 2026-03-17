import { buildEntityRoutes } from "./entity-routes";

/**
 * `strait runs` operational command group.
 */
export const runsRoutes = buildEntityRoutes({
  groupName: "runs",
  groupBrief: "Run management commands",
  basePath: "/v1/runs",
  idPlaceholder: "runID",
  idCandidates: ["id", "run_id", "status", "job_id"],
  listBrief: "List runs",
  getBrief: "Get one run",
  supportProjectFilter: true,
});
