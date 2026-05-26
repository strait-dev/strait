# Strong Cache Consistency Audit

This matrix is the source of truth for namespaces that must reject stale cache
fills across replicas. Keep it in sync with `internal/cache/strong_policy.go`.

| Namespace | Key | Version source | Mutation paths | Sync path | Cachebus path | CDC repair | Failure mode | Tests |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `authn_keys` | `key_hash` | `api_keys.cache_version` | API key create, revoke, rotate, rotated-key transaction | Strong write-through for create/new key; version barrier for revoke/old key | update/invalidate `authn_keys` | `api_keys` | Fail closed; DB confirm before allow when freshness is uncertain | `TestStrongAPIKeyCache*` |
| `permission` | `project_id + user_id` | `cache_namespace_versions(permission, key)` | role create/update/delete; member assign/remove; resource/tag policy create/delete; system role seed | Strong invalidation barrier per affected user or project barrier | invalidate `permission` / `permission_project` | RBAC tables | Fail open to DB | `TestStrongPermissionCache*` |
| `quota` | `project_id` | `project_quotas.cache_version` | quota upserts and project quota setting updates | Strong write-through or barrier invalidation | update/invalidate `quota` | `project_quotas` | Fail open to DB | `TestStrongQuotaCache*` |
| `billing_org_limits` | `org_id` | `organization_subscriptions.cache_version` | subscription plan/status/payment/add-on/spending-limit mutations | Strong barrier invalidation after subscription mutation | invalidate `billing_org_limits` | `organization_subscriptions` | Existing billing limit fail-closed paths stay fail-closed; other reads fail open to DB | `TestStrongOrgLimitCache*` |
| `worker_job` | `job_id` | `jobs.cache_version` | job create/update/delete and dispatch-config mutations | Strong write-through for create/update; barrier for delete | update/invalidate `worker_job` | `jobs` | Fail open to DB | `TestStrongWorkerJobCache*` |
| `api_job_dependencies` | `job_id + limit + cursor` | parent `jobs.cache_version` | dependency create/delete | Strong refresh or barrier after dependency-list version bump | update/invalidate `api_job_dependencies` | `job_dependencies` | Fail open to DB | `TestStrongJobDependencyCache*` |

Immutable caches such as versioned job definitions and workflow definitions are
not listed here because the version is part of the key and stale overwrites are
not possible.
