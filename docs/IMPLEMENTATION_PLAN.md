# Implementation Plan: Versioning, RBAC, Tags & Audit

> **Status: All 7 phases implemented.** See [NEXT_STEPS.md](./NEXT_STEPS.md) for remaining gaps and improvements.
>
> - [x] Phase 1: Atomic versioning (migration 049, `GetJobAtVersion`, executor wired)
> - [x] Phase 2: API key scopes (14 scopes, `requirePermission` middleware, all routes)
> - [x] Phase 3: Actor identity (migration 050, `known_actors`, audit columns, async sync)
> - [x] Phase 4: RBAC (migration 051, 4 system roles, custom roles, member management, permission cache)
> - [x] Phase 5: Version IDs (migration 052, nanoid `ver_` prefix, unique per update)
> - [x] Phase 6: Version policy (migration 053, pin/latest/minor stored — enforcement at dequeue pending)
> - [x] Phase 7: Tags everywhere (migration 054, JSONB + GIN indexes on workflows/runs)

## On Casdoor

**Skip it.** Casdoor is a full IAM server (identity + OAuth + RBAC + admin UI). It's designed to be the *sole* auth provider — it replaces Better Auth, not complements it. If you used Casdoor, you'd be running a separate Java/Go service with its own Postgres database, its own session management, its own UI. You'd have the same "two databases" problem plus a heavy dependency you don't control.

Your situation is simpler: Better Auth handles identity in the app, your Go service handles authorization internally. The actor-header pattern gives you RBAC without any external dependency. The RBAC code is ~300-400 lines of Go — not worth pulling in a full IAM server for.

---

## Pre-flight: Checks to Run Before Each Phase Commit

```bash
# 1. Build
go build ./...

# 2. Vet
go vet ./...

# 3. Lint
golangci-lint run --timeout=5m ./...

# 4. Unit tests with race detector
go test -race -timeout=60s ./...

# 5. Integration tests (requires docker)
go test -race -tags=integration -timeout=20m ./internal/store ./internal/queue ./internal/pubsub ./internal/e2e
```

Every phase ends with all 5 passing green.

---

## Phase 1: Fix Atomic Versioning (Critical Correctness Bug)

**Priority:** 🔴 Critical — this is a live correctness issue  
**Effort:** Low  
**Dependencies:** None  

### Problem

The executor in `executor_dispatch.go` always reads the **current** job config:

```go
job, err := e.store.GetJob(ctx, run.JobID)  // reads live table, ignores run.JobVersion
```

But `run.JobVersion` was captured at enqueue time, and the `job_versions` table already has the snapshot. A mid-flight job update changes endpoint URLs, timeouts, and retry behavior for runs that were already queued under the old config.

### Changes

#### 1.1 — New store method: `GetJobAtVersion`

**File:** `internal/store/job_versions.go`

Add a method that reads from `job_versions` for a specific version, falling back to the live `jobs` table if no snapshot exists (version 1 runs created before snapshotting was added, or if the snapshot was never created).

```go
func (q *Queries) GetJobAtVersion(ctx context.Context, jobID string, version int) (*domain.Job, error)
```

This returns a `*domain.Job` (not `*domain.JobVersion`) so the executor doesn't need any type changes. Map the `JobVersion` fields onto a `Job` struct — the `JobVersion` table already has all the fields the executor needs (endpoint_url, max_attempts, timeout_secs, tags, etc).

Missing fields in `job_versions` that exist on `jobs`: `max_concurrency`, `execution_window_cron`, `timezone`, `rate_limit_max`, `rate_limit_window_secs`, `dedup_window_secs`, `enabled`, `retry_strategy`, `retry_delays_secs`, `environment_id`, `group_id`. These need a migration to add to `job_versions`.

#### 1.2 — Migration: Add missing columns to `job_versions`

**File:** `migrations/000049_job_versions_full_snapshot.up.sql`

```sql
ALTER TABLE job_versions
    ADD COLUMN IF NOT EXISTS max_concurrency INT,
    ADD COLUMN IF NOT EXISTS execution_window_cron TEXT,
    ADD COLUMN IF NOT EXISTS timezone TEXT,
    ADD COLUMN IF NOT EXISTS rate_limit_max INT,
    ADD COLUMN IF NOT EXISTS rate_limit_window_secs INT,
    ADD COLUMN IF NOT EXISTS dedup_window_secs INT,
    ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS retry_strategy TEXT,
    ADD COLUMN IF NOT EXISTS retry_delays_secs INT[],
    ADD COLUMN IF NOT EXISTS environment_id TEXT,
    ADD COLUMN IF NOT EXISTS group_id TEXT,
    ADD COLUMN IF NOT EXISTS fallback_endpoint_url TEXT;
```

Update the `UpdateJob` CTE in `jobs.go` to snapshot these new columns too.

#### 1.3 — Wire executor to use versioned config

**File:** `internal/worker/executor_dispatch.go`

Change `execute()`:

```go
// Before:
job, err := e.store.GetJob(ctx, run.JobID)

// After:
job, err := e.store.GetJobAtVersion(ctx, run.JobID, run.JobVersion)
```

#### 1.4 — Add `GetJobAtVersion` to executor store interface

**File:** `internal/worker/executor.go`

Add to the `ExecutorStore` interface:

```go
GetJobAtVersion(ctx context.Context, jobID string, version int) (*domain.Job, error)
```

#### 1.5 — Update mock in executor tests

**File:** `internal/worker/executor_test.go`

Add `getJobAtVersionFn` to `mockExecutorStore`. Update existing tests to use it. Write new tests:

### Tests

| Test | What it verifies |
|---|---|
| `TestExecute_UsesVersionedJobConfig` | Enqueue at v1 (endpoint=A), update job to v2 (endpoint=B), execute → dispatches to A |
| `TestExecute_FallsBackToLiveJob` | Run with version that has no snapshot → falls back to live job table |
| `TestGetJobAtVersion_ReturnsSnapshot` | Store unit test: create job, update it, get at v1 → returns original config |
| `TestGetJobAtVersion_MapsAllFields` | All `Job` fields roundtrip through `JobVersion` correctly |
| `TestUpdateJob_SnapshotsNewColumns` | Update job with max_concurrency, environment_id, etc → snapshot has them |

Integration tests in `store_integration_test.go`:

| Test | What it verifies |
|---|---|
| `TestIntegration_GetJobAtVersion` | Full roundtrip: create → update → GetJobAtVersion(v1) returns original |
| `TestIntegration_GetJobAtVersion_NoSnapshot` | GetJobAtVersion for version with no snapshot → falls back to live |
| `TestIntegration_UpdateJob_FullSnapshot` | All new columns appear in job_versions after update |

### Commit

```
feat: fix atomic versioning — executor reads versioned job config

The executor now reads job configuration from the job_versions snapshot
table using the version captured at enqueue time, instead of always
reading the live jobs table. This prevents mid-flight job updates from
affecting already-queued runs.

- Add GetJobAtVersion store method with fallback to live table
- Migration 049: add missing columns to job_versions for full snapshot
- Update executor to use run.JobVersion for config resolution
- Update UpdateJob CTE to snapshot all columns
```

---

## Phase 2: Enforce API Key Scopes (Quick Win)

**Priority:** Medium (unblocks RBAC thinking)  
**Effort:** Low  
**Dependencies:** None  

### Problem

`APIKey.Scopes` is stored in the database and returned in API responses but **never checked**. Any API key can do anything.

### Changes

#### 2.1 — Define scope constants

**File:** `internal/domain/scopes.go` (new)

```go
package domain

const (
    ScopeAll             = "*"
    ScopeJobsRead        = "jobs:read"
    ScopeJobsWrite       = "jobs:write"
    ScopeJobsTrigger     = "jobs:trigger"
    ScopeRunsRead        = "runs:read"
    ScopeRunsWrite       = "runs:write"
    ScopeWorkflowsRead   = "workflows:read"
    ScopeWorkflowsWrite  = "workflows:write"
    ScopeWorkflowsTrigger = "workflows:trigger"
    ScopeSecretsRead     = "secrets:read"
    ScopeSecretsWrite    = "secrets:write"
    ScopeAPIKeysManage   = "api-keys:manage"
)

var ValidScopes = map[string]bool{ ... }

func ValidateScopes(scopes []string) error { ... }
```

#### 2.2 — Add scope to context + `requireScope` middleware

**File:** `internal/api/middleware.go`

- In `apiKeyAuth`: put `apiKey.Scopes` into context
- New function `requireScope(scope string) func(http.Handler) http.Handler`
- Empty scopes or `["*"]` = full access (backwards compatible with existing keys)

#### 2.3 — Wire scopes to route groups

**File:** `internal/api/routes.go`

```go
r.Route("/jobs", func(r chi.Router) {
    r.With(requireScope(domain.ScopeJobsWrite)).Post("/", s.handleCreateJob)
    r.With(requireScope(domain.ScopeJobsRead)).Get("/", s.handleListJobs)
    // ...
    r.With(requireScope(domain.ScopeJobsTrigger)).Post("/{jobID}/trigger", s.handleTriggerJob)
})
```

#### 2.4 — Validate scopes on API key creation

**File:** `internal/api/api_keys.go`

Validate that requested scopes are from `ValidScopes`. Reject unknown scopes.

### Tests

| Test | What it verifies |
|---|---|
| `TestRequireScope_AllowsWildcard` | Key with `["*"]` passes any scope check |
| `TestRequireScope_AllowsMatchingScope` | Key with `["jobs:read"]` passes `jobs:read` check |
| `TestRequireScope_BlocksMissingScope` | Key with `["jobs:read"]` gets 403 on `jobs:write` route |
| `TestRequireScope_EmptyScopesAllowAll` | Key with `[]` scopes = backwards compatible, allows all |
| `TestCreateAPIKey_ValidatesScopes` | Creating key with `["invalid:scope"]` returns 400 |
| `TestRequireScope_MultipleScopesOnKey` | Key with `["jobs:read", "runs:read"]` allows both |

### Commit

```
feat: enforce API key scopes

API keys now have their scopes checked on every request. Keys with
empty scopes or ["*"] retain full access for backwards compatibility.
Unknown scopes are rejected at key creation time.
```

---

## Phase 3: Actor Identity + Audit Columns

**Priority:** Medium  
**Effort:** Medium  
**Dependencies:** Phase 2 (scopes in context pattern)  

### Changes

#### 3.1 — Migration: `known_actors` table + audit columns

**File:** `migrations/000050_actor_identity.up.sql`

```sql
CREATE TABLE known_actors (
    id         TEXT PRIMARY KEY,
    email      TEXT,
    name       TEXT,
    avatar_url TEXT,
    synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE jobs ADD COLUMN created_by TEXT, ADD COLUMN updated_by TEXT;
ALTER TABLE workflows ADD COLUMN created_by TEXT, ADD COLUMN updated_by TEXT;
ALTER TABLE job_runs ADD COLUMN created_by TEXT;
ALTER TABLE workflow_runs ADD COLUMN created_by TEXT;
```

#### 3.2 — Actor context extraction

**File:** `internal/api/middleware.go`

Add context keys and extraction for `X-Actor-Id`, `X-Actor-Email`, `X-Actor-Name`:

```go
const ctxActorIDKey contextKey = "actor_id"
const ctxActorTypeKey contextKey = "actor_type" // "user" or "api_key"

func actorFromContext(ctx context.Context) string { ... }
```

In `apiKeyAuth`, after validation:
- If `X-Actor-Id` header present → set actor_id = header value, actor_type = "user"
- If no actor header → set actor_id = apiKey.ID, actor_type = "api_key"

This way every request has an actor — either a user (from the app) or an API key (for machine access).

#### 3.3 — Lazy actor sync

**File:** `internal/store/actors.go` (new)

```go
func (q *Queries) UpsertKnownActor(ctx context.Context, id, email, name string) error
func (q *Queries) GetKnownActor(ctx context.Context, id string) (*domain.KnownActor, error)
```

Called async from middleware (goroutine, like `TouchAPIKeyLastUsed` today).

#### 3.4 — Thread actor into store operations

Update `CreateJob`, `UpdateJob`, `CreateWorkflow`, `UpdateWorkflow`, `CreateRun`, `CreateWorkflowRun` to accept and persist `created_by`/`updated_by` from context.

Approach: update the domain structs to include `CreatedBy`/`UpdatedBy` fields, set them in API handlers from context before calling store.

#### 3.5 — Update domain types

**File:** `internal/domain/types.go`

Add to `Job`, `Workflow`:
```go
CreatedBy string `json:"created_by,omitempty"`
UpdatedBy string `json:"updated_by,omitempty"`
```

Add to `JobRun`, `WorkflowRun`:
```go
CreatedBy string `json:"created_by,omitempty"`
```

Add new type:
```go
type KnownActor struct {
    ID        string    `json:"id"`
    Email     string    `json:"email,omitempty"`
    Name      string    `json:"name,omitempty"`
    AvatarURL string    `json:"avatar_url,omitempty"`
    SyncedAt  time.Time `json:"synced_at"`
}
```

### Tests

| Test | What it verifies |
|---|---|
| `TestApiKeyAuth_ExtractsActorFromHeader` | Request with `X-Actor-Id` → context has actor_id |
| `TestApiKeyAuth_FallsBackToKeyID` | No actor header → context actor = API key ID |
| `TestApiKeyAuth_ActorTypeUser` | With `X-Actor-Id` → actor_type = "user" |
| `TestApiKeyAuth_ActorTypeAPIKey` | Without `X-Actor-Id` → actor_type = "api_key" |
| `TestCreateJob_SetsCreatedBy` | Create job with actor in context → job.created_by set |
| `TestUpdateJob_SetsUpdatedBy` | Update job with actor in context → job.updated_by set |
| `TestTriggerJob_SetsCreatedBy` | Trigger job → run.created_by set |
| `TestUpsertKnownActor_InsertsNew` | First sync → row created |
| `TestUpsertKnownActor_UpdatesExisting` | Second sync with new email → row updated |

Integration tests:
| Test | What it verifies |
|---|---|
| `TestIntegration_AuditColumns` | Full flow: create job → check created_by, update → check updated_by |
| `TestIntegration_KnownActors` | Upsert → get → verify roundtrip |

### Commit

```
feat: actor identity and audit columns

Requests from the app include X-Actor-Id, X-Actor-Email, X-Actor-Name
headers. The orchestrator extracts the actor and stores it as
created_by/updated_by on jobs, workflows, and runs.

Actor profile data is lazily synced to a known_actors table for
display purposes. Machine requests (no actor header) use the API
key ID as the actor.
```

---

## Phase 4: RBAC — Roles & Permissions

**Priority:** Medium  
**Effort:** Medium-High  
**Dependencies:** Phase 3 (actor identity in context)  

### Changes

#### 4.1 — Migration: RBAC tables

**File:** `migrations/000051_rbac.up.sql`

```sql
CREATE TABLE project_roles (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT,
    permissions TEXT[] NOT NULL DEFAULT '{}',
    is_system   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, name)
);

CREATE TABLE project_member_roles (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    user_id     TEXT NOT NULL,
    role_id     TEXT NOT NULL REFERENCES project_roles(id) ON DELETE CASCADE,
    granted_by  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, user_id)
);

CREATE INDEX idx_member_roles_user ON project_member_roles(user_id);
CREATE INDEX idx_member_roles_project ON project_member_roles(project_id);

-- Optional: per-resource policies for fine-grained control
CREATE TABLE resource_policies (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL,
    resource_type  TEXT NOT NULL,
    resource_id    TEXT NOT NULL,
    user_id        TEXT NOT NULL,
    actions        TEXT[] NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (resource_type, resource_id, user_id)
);

CREATE INDEX idx_resource_policies_resource ON resource_policies(resource_type, resource_id);
CREATE INDEX idx_resource_policies_user ON resource_policies(user_id);
```

#### 4.2 — Seed default system roles

**File:** `migrations/000051_rbac.up.sql` (continued)

Insert via a function or in the app bootstrap, not hardcoded in migration. But define the default permission sets:

```
admin    → ["*"]
operator → ["jobs:read", "jobs:write", "jobs:trigger", "runs:read", "runs:write",
             "workflows:read", "workflows:write", "workflows:trigger", "secrets:read"]
viewer   → ["jobs:read", "runs:read", "workflows:read"]
triggerer → ["jobs:read", "jobs:trigger", "runs:read", "workflows:read", "workflows:trigger"]
```

#### 4.3 — RBAC store methods

**File:** `internal/store/rbac.go` (new)

```go
func (q *Queries) CreateProjectRole(ctx context.Context, role *domain.ProjectRole) error
func (q *Queries) ListProjectRoles(ctx context.Context, projectID string) ([]domain.ProjectRole, error)
func (q *Queries) AssignMemberRole(ctx context.Context, membership *domain.ProjectMemberRole) error
func (q *Queries) GetMemberRole(ctx context.Context, projectID, userID string) (*domain.ProjectMemberRole, error)
func (q *Queries) RemoveMemberRole(ctx context.Context, projectID, userID string) error
func (q *Queries) ListProjectMembers(ctx context.Context, projectID string) ([]domain.ProjectMemberRole, error)
func (q *Queries) GetUserPermissions(ctx context.Context, projectID, userID string) ([]string, error)
func (q *Queries) CreateResourcePolicy(ctx context.Context, policy *domain.ResourcePolicy) error
func (q *Queries) GetResourcePolicies(ctx context.Context, resourceType, resourceID, userID string) ([]string, error)
func (q *Queries) DeleteResourcePolicy(ctx context.Context, id string) error
```

#### 4.4 — Authorization middleware

**File:** `internal/api/middleware.go`

```go
func (s *Server) requirePermission(permission string) func(http.Handler) http.Handler
```

Logic:
1. If actor_type == "api_key" → check API key scopes (Phase 2 behavior)
2. If actor_type == "user" → load role from `project_member_roles` → check permissions
3. Cache user permissions per-request in context to avoid repeated DB hits

Replace Phase 2's `requireScope` with this unified `requirePermission` that handles both auth types.

#### 4.5 — RBAC API endpoints

**File:** `internal/api/rbac.go` (new)

```
POST   /v1/roles                    → create custom role
GET    /v1/roles                    → list roles for project
POST   /v1/members                  → assign role to user
GET    /v1/members                  → list project members
DELETE /v1/members/{userID}         → remove member
POST   /v1/resource-policies        → create per-resource policy
GET    /v1/resource-policies        → list policies for resource
DELETE /v1/resource-policies/{id}   → delete policy
```

#### 4.6 — Domain types

**File:** `internal/domain/types.go`

```go
type ProjectRole struct {
    ID          string    `json:"id"`
    ProjectID   string    `json:"project_id"`
    Name        string    `json:"name"`
    Description string    `json:"description,omitempty"`
    Permissions []string  `json:"permissions"`
    IsSystem    bool      `json:"is_system"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type ProjectMemberRole struct {
    ID        string    `json:"id"`
    ProjectID string    `json:"project_id"`
    UserID    string    `json:"user_id"`
    RoleID    string    `json:"role_id"`
    GrantedBy string    `json:"granted_by,omitempty"`
    CreatedAt time.Time `json:"created_at"`
}

type ResourcePolicy struct {
    ID           string    `json:"id"`
    ProjectID    string    `json:"project_id"`
    ResourceType string    `json:"resource_type"`
    ResourceID   string    `json:"resource_id"`
    UserID       string    `json:"user_id"`
    Actions      []string  `json:"actions"`
    CreatedAt    time.Time `json:"created_at"`
}
```

### Tests

| Test | What it verifies |
|---|---|
| `TestRequirePermission_AdminAllowsAll` | User with admin role passes any check |
| `TestRequirePermission_ViewerBlocksWrite` | Viewer role gets 403 on write endpoints |
| `TestRequirePermission_OperatorCanTrigger` | Operator can trigger jobs |
| `TestRequirePermission_APIKeyUsesScopes` | API key auth still uses scopes, not roles |
| `TestRequirePermission_UnknownUserDenied` | User with no role assigned → 403 |
| `TestResourcePolicy_OverridesRole` | Viewer + resource policy "trigger" on job X → can trigger X |
| `TestResourcePolicy_DoesNotLeakToOtherJobs` | Policy on job X doesn't grant access to job Y |
| `TestCreateRole_RejectsInvalidPermissions` | Custom role with unknown permission → 400 |
| `TestAssignRole_RejectsNonexistentRole` | Assign unknown role_id → 404 |
| `TestPermissionCache_HitsOnSecondCall` | Two checks in same request → only one DB query |

Integration tests:
| Test | What it verifies |
|---|---|
| `TestIntegration_RBAC_FullFlow` | Create role → assign to user → verify permissions → remove |
| `TestIntegration_ResourcePolicies` | Create policy → check access → delete → verify revoked |
| `TestIntegration_DefaultSystemRoles` | System roles exist with correct permissions |

### Commit

```
feat: RBAC with project roles and resource policies

Full role-based access control:
- Project roles with configurable permissions (admin, operator, viewer, custom)
- Member role assignments linking Better Auth users to project roles
- Per-resource policies for fine-grained control (e.g., "user X can trigger job Y")
- Unified permission middleware handling both API key scopes and user roles
- API endpoints for role and member management
```

---

## Phase 5: Version IDs

**Priority:** Low  
**Effort:** Medium  
**Dependencies:** Phase 1 (atomic versioning)  

### Changes

#### 5.1 — Migration: add `version_id` column

**File:** `migrations/000052_version_ids.up.sql`

```sql
ALTER TABLE jobs ADD COLUMN version_id TEXT;
ALTER TABLE workflows ADD COLUMN version_id TEXT;
ALTER TABLE job_versions ADD COLUMN version_id TEXT;
ALTER TABLE workflow_versions ADD COLUMN version_id TEXT;
ALTER TABLE job_runs ADD COLUMN job_version_id TEXT;
ALTER TABLE workflow_runs ADD COLUMN workflow_version_id TEXT;

-- Backfill existing rows
UPDATE jobs SET version_id = id || ':v' || version WHERE version_id IS NULL;
UPDATE workflows SET version_id = id || ':v' || version WHERE version_id IS NULL;
UPDATE job_versions SET version_id = id WHERE version_id IS NULL;

CREATE UNIQUE INDEX idx_jobs_version_id ON jobs(version_id) WHERE version_id IS NOT NULL;
CREATE UNIQUE INDEX idx_workflows_version_id ON workflows(version_id) WHERE version_id IS NOT NULL;
```

#### 5.2 — Generate UUIDv7 version_id on updates

**File:** `internal/store/jobs.go`, `internal/store/workflows.go`

On `CreateJob` / `UpdateJob`: generate `version_id = uuid.Must(uuid.NewV7()).String()`.
On `CreateRun`: copy `job.VersionID` → `run.JobVersionID`.

Keep the integer `version` for ordering and internal use. The `version_id` is the external-facing identifier.

#### 5.3 — Update domain types

Add `VersionID string` to `Job`, `Workflow`, `JobVersion`, `JobRun`, `WorkflowRun`.

#### 5.4 — API: accept version_id in queries

Add `GET /v1/jobs/{jobID}/versions/{versionID}` endpoint that looks up by version_id instead of integer.

### Tests

| Test | What it verifies |
|---|---|
| `TestCreateJob_GeneratesVersionID` | New job has non-empty version_id |
| `TestUpdateJob_NewVersionID` | Update job → version_id changes, version int increments |
| `TestTriggerJob_CapturesVersionID` | Run has job_version_id matching job's current version_id |
| `TestGetJobVersion_ByVersionID` | Lookup by UUIDv7 version_id returns correct snapshot |

### Commit

```
feat: unique version IDs for jobs and workflows

Add UUIDv7-based version_id alongside the integer version counter.
The integer stays for ordering; the version_id is the public-facing
unique identifier captured on runs for exact provenance tracking.
```

---

## Phase 6: Smart Version Detection

**Priority:** Medium  
**Effort:** Medium  
**Dependencies:** Phase 1 (atomic versioning), Phase 5 (version IDs, optional)  

### Changes

#### 6.1 — Add `version_policy` to jobs and workflows

**File:** `migrations/000053_version_policy.up.sql`

```sql
ALTER TABLE jobs ADD COLUMN version_policy TEXT NOT NULL DEFAULT 'pin';
ALTER TABLE workflows ADD COLUMN version_policy TEXT NOT NULL DEFAULT 'pin';
```

Values: `"pin"` (run uses enqueue-time version) | `"latest"` (upgrade to latest on dequeue) | `"minor"` (upgrade if backwards-compatible flag set)

#### 6.2 — Add `backwards_compatible` flag to versions

```sql
ALTER TABLE job_versions ADD COLUMN backwards_compatible BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE workflow_versions ADD COLUMN backwards_compatible BOOLEAN NOT NULL DEFAULT FALSE;
```

Set by the user when updating: "this change is safe for in-flight runs."

#### 6.3 — Dequeue-time version upgrade

**File:** `internal/queue/postgres.go`

In `Dequeue` / `DequeueN`, after claiming the run, if job's `version_policy = "latest"`:

```go
// After dequeue, before returning:
if job.VersionPolicy == "latest" {
    run.JobVersion = job.Version  // upgrade to current
} else if job.VersionPolicy == "minor" && latestVersion.BackwardsCompatible {
    run.JobVersion = job.Version
}
// else: keep run.JobVersion as-is (pin behavior)
```

#### 6.4 — Domain type updates

Add `VersionPolicy string` to `Job` and `Workflow`.
Add `BackwardsCompatible bool` to `JobVersion`.

### Tests

| Test | What it verifies |
|---|---|
| `TestDequeue_PinPolicy_KeepsOriginalVersion` | Enqueue at v1, update to v2, dequeue → run.JobVersion = 1 |
| `TestDequeue_LatestPolicy_UpgradesToCurrent` | Enqueue at v1, update to v2 (policy=latest), dequeue → version = 2 |
| `TestDequeue_MinorPolicy_UpgradesIfCompatible` | v2 marked backwards_compatible → upgrades |
| `TestDequeue_MinorPolicy_KeepsIfIncompatible` | v2 not backwards_compatible → keeps v1 |
| `TestUpdateJob_CanSetVersionPolicy` | Update job with version_policy field persists correctly |
| `TestTriggerJob_RespectsVersionPolicy` | API trigger with policy=latest enqueues with current version |

### Commit

```
feat: smart version detection for queued runs

Jobs and workflows can configure version_policy:
- "pin": runs execute with the version they were enqueued with (default, safe)
- "latest": queued runs upgrade to the latest version at dequeue time
- "minor": upgrade only if the new version is marked backwards_compatible

This gives operators control over how deployments affect in-flight work.
```

---

## Phase 7: Tags Everywhere

**Priority:** Medium  
**Effort:** Low-Medium  
**Dependencies:** None (can run in parallel with any phase)  

### Changes

#### 7.1 — Migration: add tags to remaining tables

**File:** `migrations/000054_tags_everywhere.up.sql`

```sql
-- Workflows already have no tags
ALTER TABLE workflows ADD COLUMN tags JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Workflow runs
ALTER TABLE workflow_runs ADD COLUMN tags JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Job runs (formalize alongside existing metadata)
ALTER TABLE job_runs ADD COLUMN tags JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Indexes for tag queries
CREATE INDEX idx_workflows_tags ON workflows USING gin(tags);
CREATE INDEX idx_workflow_runs_tags ON workflow_runs USING gin(tags);
CREATE INDEX idx_job_runs_tags ON job_runs USING gin(tags);
CREATE INDEX idx_jobs_tags ON jobs USING gin(tags);
```

#### 7.2 — Domain type updates

Add `Tags map[string]string` to `Workflow`, `WorkflowRun`, `JobRun`.

#### 7.3 — Store methods: tag-based queries

**File:** `internal/store/workflows.go`

```go
func (q *Queries) ListWorkflowsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.Workflow, error)
```

**File:** `internal/store/runs.go`

```go
func (q *Queries) ListRunsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.JobRun, error)
```

**File:** `internal/store/workflow_runs.go`

```go
func (q *Queries) ListWorkflowRunsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error)
```

Follow the exact pattern from the existing `ListJobsByTag`.

#### 7.4 — API: tag query parameters

Update list endpoints to accept `tag_key` and `tag_value` query parameters:
- `GET /v1/workflows?tag_key=team&tag_value=platform`
- `GET /v1/runs?tag_key=environment&tag_value=production`
- `GET /v1/workflow-runs?tag_key=release&tag_value=v2.1`

#### 7.5 — API: accept tags on create/update

Update `handleCreateWorkflow`, `handleUpdateWorkflow`, `handleTriggerJob` (for run tags), `handleTriggerWorkflow` (for workflow run tags) to accept a `tags` field.

#### 7.6 — Tag propagation

When a job run is created via trigger, optionally inherit tags from the job. When a workflow run is created, optionally inherit tags from the workflow. Tags passed at trigger time override inherited ones.

### Tests

| Test | What it verifies |
|---|---|
| `TestCreateWorkflow_WithTags` | Tags persist and roundtrip |
| `TestListWorkflowsByTag_KeyOnly` | Filter by tag key presence |
| `TestListWorkflowsByTag_KeyValue` | Filter by exact key=value |
| `TestTriggerJob_WithRunTags` | Run gets tags from trigger request |
| `TestTriggerJob_InheritsJobTags` | Run inherits job tags when none specified |
| `TestTriggerJob_OverridesInheritedTags` | Trigger tags override job tags |
| `TestListRuns_FilterByTag` | List runs with tag filter returns correct subset |
| `TestListWorkflowRuns_FilterByTag` | Same for workflow runs |

Integration tests:
| Test | What it verifies |
|---|---|
| `TestIntegration_WorkflowTags` | Create with tags → list by tag → verify |
| `TestIntegration_RunTags` | Trigger with tags → list by tag → verify |
| `TestIntegration_TagInheritance` | Job tags propagate to runs |

### Commit

```
feat: tags on workflows, runs, and workflow runs

Tags (key-value pairs) can now be set on all major entities:
- Workflows: set at create/update time
- Job runs: set at trigger time, inherited from job if not specified
- Workflow runs: set at trigger time, inherited from workflow

All list endpoints support tag_key/tag_value query parameters for
filtering. GIN indexes on JSONB columns for efficient tag queries.
```

---

## Phase Summary

```
Phase 1 → Atomic versioning fix         (Critical, no deps)
Phase 2 → Enforce API key scopes        (Quick win, no deps)
Phase 3 → Actor identity + audit         (Needs Phase 2 pattern)
Phase 4 → Full RBAC                      (Needs Phase 3)
Phase 5 → Version IDs                    (Needs Phase 1)
Phase 6 → Smart version detection        (Needs Phase 1)
Phase 7 → Tags everywhere               (No deps, parallel)
```

```
Timeline (can overlap):

Phase 1 ████░░░░░░░░░░░░░░░░  ← Do first, correctness bug
Phase 2 ░░██░░░░░░░░░░░░░░░░  ← Quick win
Phase 7 ░░░███░░░░░░░░░░░░░░  ← Independent, can start early
Phase 3 ░░░░░████░░░░░░░░░░░  ← After Phase 2
Phase 5 ░░░░░████░░░░░░░░░░░  ← After Phase 1, parallel with 3
Phase 4 ░░░░░░░░░██████░░░░░  ← Biggest phase, after Phase 3
Phase 6 ░░░░░░░░░░░░░████░░░  ← After Phase 1
```

Each phase: implement → test → `go build && go vet && golangci-lint run && go test -race ./... && go test -race -tags=integration ./internal/store ./internal/queue ./internal/pubsub ./internal/e2e` → commit.
