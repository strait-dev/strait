package cdc

var requiredConsumerTables = []string{
	"public.api_keys",
	"public.project_roles",
	"public.project_member_roles",
	"public.resource_policies",
	"public.tag_policies",
	"public.project_quotas",
	"public.organization_subscriptions",
	"public.jobs",
	"public.job_dependencies",
	"public.job_runs",
	"public.workflow_runs",
	"public.workflow_step_runs",
	"public.event_triggers",
}

// RequiredConsumerTables returns the database tables that the runtime Sequin consumer must subscribe to.
func RequiredConsumerTables() []string {
	tables := make([]string, len(requiredConsumerTables))
	copy(tables, requiredConsumerTables)
	return tables
}
