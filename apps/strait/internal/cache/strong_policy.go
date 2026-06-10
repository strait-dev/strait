package cache

// StrongNamespacePolicy documents and validates the consistency contract for a
// cache namespace that must not serve stale authorization or quota decisions.
type StrongNamespacePolicy struct {
	Namespace        string
	CacheKey         string
	VersionSource    string
	MutationPaths    []string
	WriteThroughPath string
	BusPath          string
	CDCRepairPath    string
	CDCTables        []string
	FailureMode      string
	TestMarker       string
}

// StrongNamespacePolicies is the executable counterpart to the human-readable
// audit matrix. Keep this list in sync with docs/cache-strong-consistency.md.
var StrongNamespacePolicies = []StrongNamespacePolicy{
	{
		Namespace:     "authn_keys",
		CacheKey:      "key_hash",
		VersionSource: "api_keys.cache_version",
		MutationPaths: []string{
			"handleCreateAPIKey",
			"handleRevokeAPIKey",
			"handleRotateAPIKey",
			"store.CreateRotatedAPIKey",
		},
		WriteThroughPath: "apiKeyCache.Set / apiKeyCache.Invalidate via StrongWriteThrough or StrongInvalidate",
		BusPath:          "cachebus update/invalidate namespace authn_keys",
		CDCRepairPath:    "cdc.NewCacheInvalidationHandlers(api_keys)",
		CDCTables:        []string{"api_keys"},
		FailureMode:      "fail-closed: DB confirmation before allow when freshness is uncertain",
		TestMarker:       "TestStrongAPIKeyCache",
	},
	{
		Namespace:     "permission",
		CacheKey:      "project_id + user_id",
		VersionSource: "cache_namespace_versions(permission, project_id/user_id)",
		MutationPaths: []string{
			"role create/update/delete",
			"member assign/remove",
			"resource policy create/delete",
			"tag policy create/delete",
			"system role seed",
		},
		WriteThroughPath: "permissionCache.SetWithVersion / permissionCache.InvalidateWithVersion",
		BusPath:          "cachebus invalidate namespace permission and permission_project",
		CDCRepairPath: "cdc.NewCacheInvalidationHandlers(" +
			"project_roles, project_member_roles, resource_policies, tag_policies)",
		CDCTables: []string{
			"project_roles",
			"project_member_roles",
			"resource_policies",
			"tag_policies",
		},
		FailureMode: "fail-open to DB",
		TestMarker:  "TestStrongPermissionCache",
	},
	{
		Namespace:        "quota",
		CacheKey:         "project_id",
		VersionSource:    "project_quotas.cache_version",
		MutationPaths:    []string{"UpdateProjectMaxKeyLifetimeDays", "project quota upserts", "quota admin mutations"},
		WriteThroughPath: "quotaCache.SetWithVersion / quotaCache.InvalidateWithVersion",
		BusPath:          "cachebus update/invalidate namespace quota",
		CDCRepairPath:    "cdc.NewCacheInvalidationHandlers(project_quotas)",
		CDCTables:        []string{"project_quotas"},
		FailureMode:      "fail-open to DB",
		TestMarker:       "TestStrongQuotaCache",
	},
	{
		Namespace:     "billing_org_limits",
		CacheKey:      "org_id",
		VersionSource: "organization_subscriptions.cache_version",
		MutationPaths: []string{
			"subscription create/update/delete",
			"plan/status/payment/add-on changes",
			"spending limit changes",
		},
		WriteThroughPath: "Enforcer.InvalidateOrgCacheWithVersion",
		BusPath:          "cachebus invalidate namespace billing_org_limits",
		CDCRepairPath:    "cdc.NewCacheInvalidationHandlers(organization_subscriptions)",
		CDCTables:        []string{"organization_subscriptions"},
		FailureMode:      "fail-open to DB except billing fail-closed limit paths already marked fail-closed",
		TestMarker:       "TestStrongOrgLimitCache",
	},
	{
		Namespace:     "worker_job",
		CacheKey:      "job_id",
		VersionSource: "jobs.cache_version",
		MutationPaths: []string{
			"handleCreateJob",
			"handleUpdateJob",
			"handleDeleteJob",
			"job state mutations that change dispatch config",
		},
		WriteThroughPath: "worker job tier StrongWriteThrough / StrongInvalidate",
		BusPath:          "cachebus update/invalidate namespace worker_job",
		CDCRepairPath:    "cdc.NewCacheInvalidationHandlers(jobs)",
		CDCTables:        []string{"jobs"},
		FailureMode:      "fail-open to DB",
		TestMarker:       "TestStrongWorkerJobCache",
	},
	{
		Namespace:     "api_trigger_job",
		CacheKey:      "job_id",
		VersionSource: "jobs.cache_version",
		MutationPaths: []string{
			"handleCreateJob",
			"handleUpdateJob",
			"handleDeleteJob",
			"handlePauseJob",
			"handleResumeJob",
			"handleSetJobEndpoint",
			"batch enable/disable jobs",
		},
		WriteThroughPath: "trigger job tier StrongInvalidate",
		BusPath:          "cachebus invalidate namespace api_trigger_job",
		CDCRepairPath:    "cdc.NewCacheInvalidationHandlers(jobs)",
		CDCTables:        []string{"jobs"},
		FailureMode:      "fail-open to DB",
		TestMarker:       "TestStrongAPITriggerJobCache",
	},
	{
		Namespace:     "api_job_dependencies",
		CacheKey:      "job_id + page limit + cursor",
		VersionSource: "jobs.cache_version as dependency-list version",
		MutationPaths: []string{
			"handleCreateJobDependency",
			"handleDeleteJobDependency",
			"store.CreateJobDependency",
			"store.DeleteJobDependency",
		},
		WriteThroughPath: "jobDependencyCache.RefreshJob or StrongInvalidate",
		BusPath:          "cachebus update/invalidate namespace api_job_dependencies",
		CDCRepairPath:    "cdc.NewCacheInvalidationHandlers(job_dependencies)",
		CDCTables:        []string{"job_dependencies"},
		FailureMode:      "fail-open to DB",
		TestMarker:       "TestStrongJobDependencyCache",
	},
}
