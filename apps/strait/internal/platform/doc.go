// Package platform holds product-neutral primitives that are shared across
// all Strait products (Jobs, Agents, and future products).
//
// The key concepts that live here are:
//
//   - Projects (internal/platform/projects): the top-level tenancy unit. A
//     project belongs to an org and scopes every product concern. Jobs and
//     Agents both live under a project; neither product owns the concept.
//
//   - Environments (internal/platform/environments): the deploy-time bucket
//     (dev/staging/prod) that any product can pin its artifacts to. Jobs bind
//     to environments via Job.EnvironmentID; Agent deployments bind via
//     AgentDeployment.EnvironmentID.
//
// Rule of thumb: if a concept must exist for a customer to use *any* Strait
// product, it belongs in internal/platform. If it only makes sense for Jobs
// or only for Agents, it belongs in that product's package (internal/jobs or
// internal/agents).
//
// Billing and quotas are explicitly *not* platform primitives: Jobs and
// Agents are separately subscribable products, and each maintains its own
// plan tier, meters, and quotas. See internal/billing for the product-scoped
// entitlement helpers (HasJobsSubscription, HasAgentsSubscription,
// GetJobsPlanForProject, GetAgentPlanForProject).
package platform
