import { buildEntityRoutes } from "./entity-routes";

/**
 * `strait events` operational command group.
 */
export const eventsRoutes = buildEntityRoutes({
  groupName: "events",
  groupBrief: "Event operations commands",
  basePath: "/v1/events",
  idPlaceholder: "eventKey",
  idCandidates: ["event_key", "id", "name", "source_type"],
  listBrief: "List events",
  getBrief: "Get one event",
  supportProjectFilter: true,
});
