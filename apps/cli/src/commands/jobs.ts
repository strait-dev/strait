import { buildEntityRoutes } from "./entity-routes";

/**
 * `strait jobs` operational command group.
 */
export const jobsRoutes = buildEntityRoutes({
  groupName: "jobs",
  groupBrief: "Job management commands",
  basePath: "/v1/jobs",
  idPlaceholder: "jobID",
  idCandidates: ["id", "job_id", "slug", "name"],
  listBrief: "List jobs",
  getBrief: "Get one job",
  supportProjectFilter: true,
});
