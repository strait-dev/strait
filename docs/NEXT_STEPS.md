# Next Steps — Post-PR #8

This document covers three areas:
1. **Implementation gaps** — features that were designed but not fully wired
2. **Improvements** — things that work but could be better
3. **Documentation** — what needs updating

---

## 1. Implementation Gaps

### 1.1 🔴 Version policy not wired to queue/worker (Critical)

**Status:** Schema + store persist `version_policy` and `backwards_compatible`, but neither the queue (`internal/queue/postgres.go`) nor the worker/executor reads them.

**What's missing:**
- `Dequeue`/`DequeueN` should check the job's `version_policy` after claiming a run
- If `latest` → upgrade `run.JobVersion` to current `job.Version`
- If `minor` → upgrade only if latest version has `backwards_compatible = true`
- If `pin` → no change (current default behavior)

**Files to change:**
- `internal/queue/postgres.go` — add post-dequeue version upgrade logic
- `internal/worker/executor.go` — no change (already reads `run.JobVersion`)
- Need to add `GetJob` or a lightweight version check to the queue's dequeue path

**Risk:** Low — `pin` is the default, so current behavior is correct. But `latest` and `minor` are effectively no-ops right now.

### 1.2 🟡 Resource policy API endpoints missing (Medium)

**Status:** Store methods exist (`CreateResourcePolicy`, `GetResourcePolicies`, `DeleteResourcePolicy`, `ListResourcePolicies`) but no API handlers or routes.

**What's missing:**
```
POST   /v1/resource-policies       → handleCreateResourcePolicy
GET    /v1/resource-policies       → handleListResourcePolicies  
DELETE /v1/resource-policies/{id}  → handleDeleteResourcePolicy
```

**Also:** `requirePermission` middleware only checks role permissions — it never consults `resource_policies` table. The plan (Phase 4) specified that resource policies should override/extend role permissions, but this isn't implemented.

### 1.3 🟡 Tag-based list endpoints not wired for runs/workflow-runs (Medium)

**Status:** Store methods exist (`ListRunsByTag`, `ListWorkflowRunsByTag`) but:
- API handlers `handleListRuns` and `handleListWorkflowRunsByProject` don't read `tag_key`/`tag_value` query params for runs
- No store interface method on `RunStore` or `WorkflowStore` for tag-based queries
- `ListRunsByTag` and `ListWorkflowRunsByTag` are on `Queries` but not exposed through the API layer

**What's missing:**
- Add `tag_key`/`tag_value` query param handling to `handleListRuns` and `handleListWorkflowRunsByProject`
- Add `ListRunsByTag` to `RunStore` interface and mock
- Add `ListWorkflowRunsByTag` to `WorkflowStore` interface and mock

### 1.4 🟡 Tag inheritance on trigger (Medium)

**Status:** The plan specified that triggering a job should inherit the job's tags onto the run (unless overridden). This isn't implemented:
- `handleTriggerJob` doesn't accept a `tags` field
- `handleTriggerWorkflow` doesn't accept a `tags` field
- No tag propagation from job → run or workflow → workflow_run

### 1.5 🟡 `ListWorkflowsByTag` not wired to API (Medium)

**Status:** Store method exists, but `handleListWorkflows` doesn't read `tag_key`/`tag_value` query params. Same pattern as jobs (which does work via `handleListJobs`).

### 1.6 🟢 `GetJobVersion` by version_id endpoint missing (Low)

**Status:** The plan (Phase 5) specified `GET /v1/jobs/{jobID}/versions/{versionID}` to look up by nanoid version_id. Not implemented — only integer version listing exists via `handleListJobVersions`.

### 1.7 🟢 System roles not seeded per-project (Low)

**Status:** `SystemRolePermissions` map exists in code but system roles are never auto-created in the database when a project is created. They exist only as a reference for what permissions each role name should have. `GetUserPermissions` reads from the DB, so if no system role rows exist for a project, users assigned to "admin" won't have permissions.

**Options:**
- Seed system roles on first API call per project (lazy)
- Seed via migration (static project IDs)
- Document that system roles must be created via API first

---

## 2. Improvements

### 2.1 Break up `APIStore` interface

**Status:** Already split into `JobStore`, `RunStore`, `WorkflowStore`, `AuthStore`, `RBACStore` — but the mock in `mock_test.go` is still a single 900-line struct. Consider generating mocks or using a mock library.

### 2.2 Pagination for RBAC endpoints

`ListProjectRoles` and `ListProjectMembers` return all results without limit/cursor. For projects with many members, this will be slow.

### 2.3 `HasScope` performance

Currently O(n) linear scan. Fine for ≤14 scopes, but if custom scopes are added, convert to `map[string]struct{}` lookup.

### 2.4 `updated_at` on `project_member_roles`

Table lacks `updated_at` — reassignment time isn't tracked. The `AssignMemberRole` doc comment notes this.

### 2.5 `created_by`/`updated_by` on `job_versions`

`GetJobAtVersion` reads `created_by`/`updated_by` from the live `jobs` table, not from the snapshot. If a job is updated by different users, the snapshot doesn't record who made that specific version.

### 2.6 Workflow tag query wiring

`handleListWorkflows` should support `tag_key`/`tag_value` params like `handleListJobs` does. The store method exists.

### 2.7 Permission cache TTL configuration

Currently hardcoded to 30 seconds. Should be configurable via `config.Config`.

### 2.8 Audit log table

`created_by`/`updated_by` columns track who made the last change, but there's no history. A dedicated `audit_log` table would record every mutation with actor, timestamp, and diff.

### 2.9 API key scoping to specific jobs/workflows

API keys currently scope to project-level permissions (`jobs:read` means ALL jobs in the project). There's no way to create a key that can only trigger a specific job. Resource policies partially solve this for users, but not for API keys.

### 2.10 Bulk role operations

No bulk assign/remove for members. For large teams, assigning roles one-by-one is slow.

---

## 3. Documentation Gaps

### 3.1 🔴 Authentication guide is stale (Critical)

`docs/guides/authentication.mdx` documents the old auth model:
- Doesn't mention API key scopes
- Doesn't mention actor identity headers (`X-Actor-Id`, `X-Actor-Email`, `X-Actor-Name`)
- Doesn't mention `actorType` ("user" vs "api_key")
- Doesn't explain that API key auth never honors actor headers (impersonation protection)
- Doesn't document the `rbac:manage` scope
- Doesn't mention the permission model (scopes for keys, roles for users)

### 3.2 🔴 No RBAC documentation (Critical)

Nothing documents:
- Role-based access control concept
- System roles (admin, operator, viewer, triggerer) and their permissions
- How to create custom roles
- How to assign members to roles
- How `requirePermission` middleware works
- How the permission cache works
- The relationship between API key scopes and user role permissions

### 3.3 🔴 No versioning documentation (Critical)

Nothing documents:
- `version` (integer) vs `version_id` (nanoid) distinction
- Version policy (`pin`, `latest`, `minor`)
- How `GetJobAtVersion` works (snapshot vs live fallback)
- The `backwards_compatible` flag
- When version snapshots are created (on `UpdateJob`)
- How runs capture version at enqueue time

### 3.4 🟡 Jobs concept page missing new fields (Medium)

`docs/concepts/jobs.mdx` documents the job model but is missing:
- `version_id` field
- `version_policy` field
- `created_by` / `updated_by` fields
- `backwards_compatible` on job versions

### 3.5 🟡 Security guide missing RBAC details (Medium)

`docs/guides/security.mdx` covers SSRF, rate limiting, CORS, but doesn't mention:
- API key scope enforcement
- RBAC permission checks
- Actor identity and audit trail
- Impersonation protection (API keys can't set X-Actor-Id)

### 3.6 🟡 OpenAPI spec outdated (Medium)

`docs/openapi.yaml` likely doesn't include:
- RBAC endpoints (`/v1/roles`, `/v1/members`)
- New fields on job/workflow responses (`version_id`, `version_policy`, `created_by`, `updated_by`, `tags`)
- Scope validation on API key creation
- 403 responses for scope enforcement

### 3.7 🟡 README doesn't mention RBAC or versioning (Medium)

The README's "Key Features" section should mention:
- Role-based access control with custom roles
- API key scope enforcement
- Atomic job versioning with version IDs
- Actor identity and audit trail
- Tags on all entities

### 3.8 🟢 API reference doesn't document error codes (Low)

No documentation of which endpoints return 403 (forbidden) and under what conditions.

### 3.9 🟢 No migration guide (Low)

No documentation explaining:
- How to run migrations 049-054
- What each migration does
- Any data backfill requirements
- Rollback procedures

---

## Priority Order

| Priority | Item | Effort |
|----------|------|--------|
| 1 | 3.1 Update authentication guide | 30 min |
| 2 | 3.2 Write RBAC documentation | 1 hour |
| 3 | 3.3 Write versioning documentation | 45 min |
| 4 | 1.1 Wire version policy to queue | 2 hours |
| 5 | 1.2 Add resource policy API endpoints | 1 hour |
| 6 | 1.3 Wire tag queries for runs | 30 min |
| 7 | 3.4 Update jobs concept page | 15 min |
| 8 | 3.5 Update security guide | 15 min |
| 9 | 3.7 Update README | 15 min |
| 10 | 1.4 Tag inheritance on trigger | 45 min |
| 11 | 1.5 Wire workflow tag queries | 15 min |
| 12 | 3.6 Update OpenAPI spec | 1 hour |
| 13 | 1.7 System role seeding | 30 min |
| 14 | 2.7 Configurable cache TTL | 10 min |
