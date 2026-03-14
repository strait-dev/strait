import {
  domains as generatedDomains,
  operations as generatedOperations,
} from "./generated.ts";

export type { OperationInput } from "./generated.ts";

export const domains = generatedDomains;
export const operations = generatedOperations;

export const health = domains.health;
export const sdk = domains.sdk;
export const analytics = domains.analytics;
export const apiKeys = domains.apiKeys;
export const batchOperations = domains.batchOperations;
export const environments = domains.environments;
export const eventSources = domains.eventSources;
export const eventTriggers = domains.eventTriggers;
export const jobGroups = domains.jobGroups;
export const jobs = domains.jobs;
export const logDrains = domains.logDrains;
export const other = domains.other;
export const runs = domains.runs;
export const secrets = domains.secrets;
export const stats = domains.stats;
export const uncategorized = domains.uncategorized;
export const webhooks = domains.webhooks;
export const workflowRuns = domains.workflowRuns;
export const workflows = domains.workflows;

export const events = eventTriggers;
export const sdkRuns = sdk;
