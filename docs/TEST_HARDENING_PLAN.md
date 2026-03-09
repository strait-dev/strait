# Test Hardening Plan — PR #8 Features

This plan audits every test for the features introduced in this PR, identifies exactly what is and isn't tested, and specifies every new test to write. Tests are grouped by feature area and classified by difficulty.

---

## Audit Legend

- ✅ = tested and correct
- ⚠️ = tested but the test doesn't fully verify behavior
- ❌ = not tested at all

---

## 1. Scopes & Validation (`domain/scopes.go`)

### Current Coverage
- ✅ `ValidateScopes` — empty, wildcard, valid singles, valid multiples, unknown, mixed
- ✅ `HasScope` — empty, wildcard, exact, no-match, multiple-match, multiple-no-match, wildcard-among-others

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 1.1 | `TestValidateScopes_NilSlice` | Easy | `nil` input doesn't panic, returns nil error |
| 1.2 | `TestValidateScopes_DuplicateScopes` | Easy | `["jobs:read","jobs:read"]` is valid (no dedup required) |
| 1.3 | `TestValidateScopes_AllValidScopes` | Easy | Every constant in `ValidScopes` map passes validation individually |
| 1.4 | `TestValidateScopes_EmptyStringScope` | Easy | `[""]` returns error (empty string is not a valid scope) |
| 1.5 | `TestValidateScopes_CaseSensitive` | Easy | `["Jobs:Read"]` fails (scopes are lowercase) |
| 1.6 | `TestValidateScopes_WhitespaceScope` | Easy | `[" jobs:read "]` fails (no trimming) |
| 1.7 | `TestHasScope_RequiredIsWildcard` | Edge | `HasScope(["jobs:read"], "*")` — should return false (wildcard is only matched in the slice, not as required) |
| 1.8 | `TestHasScope_RequiredEmptyString` | Edge | `HasScope(["jobs:read"], "")` — should return false |
| 1.9 | `TestHasScope_LargeNumberOfScopes` | Perf | 1000 scopes, last one matches — verify it works and is fast |

---

## 2. Version IDs / nanoid (`domain/nanoid.go`)

### Current Coverage
- ✅ Prefix, length, uniqueness (1000), valid chars

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 2.1 | `TestNewVersionID_Concurrent` | Medium | 100 goroutines each generating 100 IDs — no duplicates, no panics |
| 2.2 | `TestNewVersionID_NoUpperCase` | Easy | Verify no uppercase letters appear (alphabet is lowercase only) |
| 2.3 | `TestNewVersionID_NoAmbiguousChars` | Easy | No `l`, `I`, `O`, `0`... wait, `0` is in alphabet. Verify alphabet is what we expect |

---

## 3. Version Policy (`domain/types.go`)

### Current Coverage
- ✅ `IsValid` for pin, latest, minor, empty, invalid

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 3.1 | `TestVersionPolicy_JsonRoundtrip` | Easy | Marshal/unmarshal `VersionPolicyPin` to JSON and back |
| 3.2 | `TestSystemRolePermissions_Completeness` | Medium | Every scope constant appears in at least one system role |
| 3.3 | `TestSystemRolePermissions_AdminHasWildcard` | Easy | Admin role has exactly `["*"]` |
| 3.4 | `TestSystemRolePermissions_ViewerCannotWrite` | Easy | Viewer doesn't have any `:write` or `:trigger` scope |
| 3.5 | `TestSystemRolePermissions_OperatorHasRBACManage` | Easy | Operator has `rbac:manage` |
| 3.6 | `TestSystemRolePermissions_TriggererCannotManageKeys` | Easy | Triggerer doesn't have `api-keys:manage` or `rbac:manage` |

---

## 4. Permission Cache (`api/permission_cache.go`)

### Current Coverage
- ✅ Get/set, expiry, invalidate, project isolation

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 4.1 | `TestPermissionCache_EvictsOnExpiredRead` | Medium | Set entry, wait for expiry, Get returns miss AND len(entries) is 0 |
| 4.2 | `TestPermissionCache_ConcurrentReadWrite` | Hard | 50 goroutines reading, 50 writing, 10 invalidating — no panics, no races |
| 4.3 | `TestPermissionCache_SetOverwritesExisting` | Easy | Set twice for same key — second value wins |
| 4.4 | `TestPermissionCache_InvalidateNonexistent` | Easy | Invalidate key that doesn't exist — no panic |
| 4.5 | `TestPermissionCache_EmptyPermissionsSlice` | Edge | Set `[]string{}` (empty but non-nil) — Get returns it correctly, distinguishes from nil |
| 4.6 | `TestPermissionCache_ManyEntries` | Perf | Set 10,000 entries — verify all retrievable before TTL |
| 4.7 | `TestPermissionCache_ZeroTTL` | Edge | TTL=0 — everything expires immediately |
| 4.8 | `TestPermissionCache_KeySeparatorCollision` | Edge | `Set("a", "b:c", ...)` and `Set("a\x00b", "c", ...)` — verify they don't collide |

---

## 5. `requirePermission` Middleware (`api/middleware.go`)

### Current Coverage
- ✅ API key: wildcard, match, miss, empty, nil, multiple
- ✅ Internal secret: bare, with actor headers
- ✅ Unknown actor type rejected
- ✅ User: match, miss, no role, DB error, missing project, cache hit

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 5.1 | `TestRequirePermission_User_MissingActorID` | Easy | Scopes set + type="user" + project set but NO actor ID → 403 |
| 5.2 | `TestRequirePermission_User_WildcardPermission` | Easy | User with `["*"]` permission can access anything |
| 5.3 | `TestRequirePermission_User_CacheExpiry` | Medium | First request caches, sleep past TTL, second request hits DB again |
| 5.4 | `TestRequirePermission_User_CacheInvalidationReloads` | Medium | Cache populated → invalidate → next request hits DB |
| 5.5 | `TestRequirePermission_ChainedMiddleware` | Medium | Two requirePermission middlewares chained — both must pass |
| 5.6 | `TestRequirePermission_APIKey_WildcardScopeWithUserActorType` | Edge | Scopes=["*"], actorType="user" — user path fires, NOT api_key shortcut |
| 5.7 | `TestRequirePermission_ContextValuesNotLeaked` | Medium | Handler after middleware doesn't see values from previous request |

---

## 6. RBAC API Handlers (`api/rbac.go`)

### Current Coverage
- ✅ Create role (success, invalid scope)
- ✅ List roles, Get role (success, not found)
- ✅ Update role (success, not found, invalid scope)
- ✅ Delete role (not found)
- ✅ Assign member (success, role not found)
- ✅ List members
- ✅ Remove member (not found, cache invalidation)
- ✅ Assign member cache invalidation

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 6.1 | `TestHandleCreateRole_EmptyBody` | Easy | `{}` → 400 (name and permissions required) |
| 6.2 | `TestHandleCreateRole_EmptyPermissions` | Easy | `{"name":"x","permissions":[]}` → 400 (min=1 validation) |
| 6.3 | `TestHandleCreateRole_StoreError` | Easy | Store returns generic error → 500 |
| 6.4 | `TestHandleCreateRole_MalformedJSON` | Easy | `{invalid` → 400 |
| 6.5 | `TestHandleCreateRole_OversizedBody` | Medium | Body exceeding `maxRequestBodySize` → 400 |
| 6.6 | `TestHandleCreateRole_ResponseShape` | Medium | Verify response has all expected fields (id, project_id, name, description, permissions, is_system, created_at, updated_at) |
| 6.7 | `TestHandleDeleteRole_Success` | Easy | Normal delete returns 204 |
| 6.8 | `TestHandleDeleteRole_StoreError` | Easy | Store returns generic error → 500 |
| 6.9 | `TestHandleGetRole_StoreError` | Easy | Store returns generic error → 500 |
| 6.10 | `TestHandleListRoles_Empty` | Easy | No roles → returns `[]` not `null` |
| 6.11 | `TestHandleListRoles_StoreError` | Easy | Store error → 500 |
| 6.12 | `TestHandleUpdateRole_EmptyBody` | Easy | `{}` → 400 |
| 6.13 | `TestHandleUpdateRole_StoreError` | Easy | Store returns generic error → 500 |
| 6.14 | `TestHandleAssignMember_EmptyBody` | Easy | `{}` → 400 |
| 6.15 | `TestHandleAssignMember_MissingUserID` | Easy | `{"role_id":"x"}` → 400 |
| 6.16 | `TestHandleAssignMember_MissingRoleID` | Easy | `{"user_id":"x"}` → 400 |
| 6.17 | `TestHandleAssignMember_StoreError` | Easy | AssignMemberRole returns error → 500 |
| 6.18 | `TestHandleAssignMember_GetRoleStoreError` | Easy | GetProjectRole returns generic error (not ErrRoleNotFound) → 500 |
| 6.19 | `TestHandleAssignMember_ResponseShape` | Medium | Verify response has id, project_id, user_id, role_id, granted_by, created_at |
| 6.20 | `TestHandleListMembers_Empty` | Easy | No members → returns `[]` |
| 6.21 | `TestHandleListMembers_StoreError` | Easy | Store error → 500 |
| 6.22 | `TestHandleRemoveMember_Success` | Easy | Normal remove → 204 |
| 6.23 | `TestHandleRemoveMember_StoreError` | Easy | Generic error → 500 |

---

## 7. Actor Identity (`api/middleware.go` + `api/actor_test.go`)

### Current Coverage
- ✅ `actorFromContext` — user header, API key fallback, empty
- ✅ `mockActorSyncer` basic call
- ✅ Internal secret sets actor + calls syncer
- ✅ API key ignores actor headers

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 7.1 | `TestAPIKeyAuth_SetsAPIKeyActorID` | Medium | After API key auth, `actorFromContext` returns `"apikey:<id>"` |
| 7.2 | `TestInternalSecretAuth_NoActorHeaders` | Easy | Internal secret WITHOUT X-Actor-Id → no actor type set, passes through |
| 7.3 | `TestInternalSecretAuth_EmptyActorID` | Edge | `X-Actor-Id: ""` (empty string) → treated as no actor |
| 7.4 | `TestActorSyncer_ErrorDoesNotBlock` | Medium | Syncer returns error → request still succeeds (async, non-blocking) |
| 7.5 | `TestActorSyncer_NilSyncer` | Easy | Server created without ActorSyncer → no panic when actor headers present |
| 7.6 | `TestAPIKeyAuth_ExpiredKey` | Easy | Expired API key → 401 |
| 7.7 | `TestAPIKeyAuth_RevokedKey` | Easy | Revoked key → 401 |
| 7.8 | `TestAPIKeyAuth_InvalidBearer` | Easy | `Bearer invalid` (not `strait_` prefix) → 401 |
| 7.9 | `TestAPIKeyAuth_MissingAuthHeader` | Easy | No Authorization header → 401 |

---

## 8. Store: RBAC (`store/rbac.go`)

### Current Coverage (integration tests)
- ✅ CreateProjectRole, duplicate name error
- ✅ ListProjectRoles (3 roles)
- ✅ DeleteProjectRole (system protected)
- ✅ AssignMemberRole with upsert
- ✅ GetUserPermissions (success + no role)
- ✅ ResourcePolicy CRUD
- ✅ DeleteResourcePolicy not found, RemoveMemberRole not found
- ✅ UpdateProjectRole + system role blocked
- ✅ ListProjectMembers

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 8.1 | `TestDeleteProjectRole_CustomRole` | Easy | Non-system role deletes successfully, rows affected = 1 |
| 8.2 | `TestDeleteProjectRole_CascadesMemberRoles` | Hard | Delete role → member_roles referencing it are cascade-deleted |
| 8.3 | `TestGetMemberRole_Exists` | Easy | Insert member, GetMemberRole returns it correctly |
| 8.4 | `TestGetMemberRole_NotExists` | Easy | Returns nil, nil (not error) for non-existent |
| 8.5 | `TestGetMemberRole_GrantedByField` | Easy | AssignMemberRole with granted_by → GetMemberRole returns it |
| 8.6 | `TestAssignMemberRole_InvalidRoleFK` | Hard | Assign with non-existent role_id → FK error |
| 8.7 | `TestCreateResourcePolicy_Upsert` | Medium | Insert same resource+user twice → actions are updated, not duplicated |
| 8.8 | `TestListResourcePolicies_Empty` | Easy | No policies → returns empty slice, not nil |
| 8.9 | `TestListResourcePolicies_MultipleUsers` | Medium | 3 policies for same resource, different users → returns all 3 |
| 8.10 | `TestGetResourcePolicies_WrongUser` | Easy | Policy exists for user A, query for user B → nil |
| 8.11 | `TestListProjectRoles_OrderBySystemFirst` | Medium | Mix of system+custom roles → system roles listed first |
| 8.12 | `TestListProjectRoles_EmptyProject` | Easy | No roles for project → empty slice |
| 8.13 | `TestGetUserPermissions_WildcardRole` | Easy | Role with `["*"]` → returns `["*"]` |
| 8.14 | `TestCreateProjectRole_IDAutoGenerated` | Easy | Create with empty ID → ID is set (UUID) |
| 8.15 | `TestUpdateProjectRole_NameChange` | Easy | Update role name → verify via GetProjectRole |

---

## 9. Store: Actors (`store/actors.go`)

### Current Coverage
- ✅ UpsertKnownActor, preserve existing, not found

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 9.1 | `TestUpsertKnownActor_UpdateEmail` | Easy | Second upsert with new email → email updated |
| 9.2 | `TestUpsertKnownActor_EmptyEmailPreservesExisting` | Easy | Second upsert with empty email → original email kept |
| 9.3 | `TestUpsertKnownActor_BothEmpty` | Edge | Upsert with empty email AND empty name → no crash, previous values kept |
| 9.4 | `TestUpsertKnownActor_SyncedAtUpdates` | Medium | Two upserts → `synced_at` on second is later than first |
| 9.5 | `TestGetKnownActor_AllFields` | Easy | Insert with all fields → read back all fields including avatar_url |

---

## 10. Store: Jobs with New Fields (`store/jobs.go`)

### Current Coverage
- ✅ CreateJob sets version_id + created_by
- ✅ UpdateJob generates new version_id
- ✅ Default version_policy = "pin"

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 10.1 | `TestCreateJob_VersionIDPrefix` | Easy | version_id starts with "ver_" |
| 10.2 | `TestCreateJob_UpdatedByEmpty` | Easy | New job has empty updated_by (only set on update) |
| 10.3 | `TestUpdateJob_SetsUpdatedBy` | Easy | After update, `updated_by` is set, `created_by` unchanged |
| 10.4 | `TestUpdateJob_SnapshotsBeforeUpdate` | Medium | After UpdateJob, query job_versions for old version → snapshot exists with old values |
| 10.5 | `TestUpdateJob_VersionIncrements` | Easy | Create (v1) → update (v2) → update (v3) |
| 10.6 | `TestGetJobBySlug_IncludesNewFields` | Easy | Create job with all new fields → GetJobBySlug returns them |
| 10.7 | `TestListJobs_IncludesNewFields` | Easy | Create tagged job with created_by → ListJobs returns those fields |
| 10.8 | `TestDeleteJob_ActiveRunsBlocked` | Hard | Create job, create queued run, attempt delete → ErrJobHasActiveRuns |
| 10.9 | `TestDeleteJob_CompletedRunsAllowed` | Hard | Create job, create completed run, delete → succeeds |
| 10.10 | `TestDeleteJob_NotFound` | Easy | Delete non-existent job → ErrJobNotFound |
| 10.11 | `TestDeleteJob_Success` | Easy | Create job (no runs), delete → GetJob returns not found |
| 10.12 | `TestListJobsByGroup_IncludesNewFields` | Medium | Create job in group with tags → ListJobsByGroup returns all fields |
| 10.13 | `TestGetJobAtVersion_IncludesVersionID` | Medium | Create → update → GetJobAtVersion(v1) → returns old version_id |
| 10.14 | `TestCreateJob_CustomVersionPolicy` | Easy | Create with `version_policy: "latest"` → read back "latest" |

---

## 11. Store: Workflows with New Fields (`store/workflows.go`)

### Current Coverage
- ✅ CreateWorkflow sets version_id + created_by
- ✅ UpdateWorkflow generates new version_id + increments version

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 11.1 | `TestCreateWorkflow_TagsPersisted` | Easy | Create with tags → GetWorkflow returns them |
| 11.2 | `TestUpdateWorkflow_SnapshotsBeforeUpdate` | Medium | After UpdateWorkflow, query workflow_versions → snapshot exists |
| 11.3 | `TestUpdateWorkflow_UpdatedBySet` | Easy | Update with UpdatedBy → persisted |
| 11.4 | `TestUpdateWorkflow_TagsUpdated` | Easy | Create with tags A → update with tags B → GetWorkflow returns tags B |
| 11.5 | `TestUpdateWorkflow_NotFound` | Easy | Update non-existent → ErrWorkflowNotFound |
| 11.6 | `TestListWorkflows_IncludesNewFields` | Easy | List returns version_id, tags, created_by |
| 11.7 | `TestGetWorkflowBySlug_IncludesNewFields` | Easy | Verify version_id, tags, version_policy via slug lookup |
| 11.8 | `TestListCronWorkflows_IncludesNewFields` | Medium | Cron workflow with tags → ListCronWorkflows returns tags |

---

## 12. Store: Job Versions (`store/job_versions.go`)

### Current Coverage
- ⚠️ `scanJobVersion` recently fixed but no test verifies version_id/backwards_compatible come through

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 12.1 | `TestCreateJobVersion_WithVersionID` | Easy | Create version with version_id → persisted |
| 12.2 | `TestCreateJobVersion_BackwardsCompatible` | Easy | Create with backwards_compatible=true → field persisted |
| 12.3 | `TestListJobVersionsByJob_IncludesNewFields` | Medium | Create versions with version_id and backwards_compatible → listed correctly |
| 12.4 | `TestGetJobVersion_IncludesNewFields` | Easy | Get specific version → has version_id and backwards_compatible |
| 12.5 | `TestGetJobAtVersion_Fallback` | Medium | No snapshot for v1 → falls back to live job correctly |
| 12.6 | `TestGetJobAtVersion_SnapshotExists` | Medium | Snapshot for v1 exists → returns snapshot data, not live data |
| 12.7 | `TestListJobVersionsByJob_Ordering` | Easy | Multiple versions → listed in DESC order by version |
| 12.8 | `TestListJobVersionsByJob_Pagination` | Medium | Create 5 versions, paginate with limit=2 → correct cursor behavior |

---

## 13. Tags Queries

### Current Coverage
- ✅ ListJobsByTag key-only, key-value
- ✅ ListWorkflowsByTag match + wrong value
- ✅ Empty tags

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 13.1 | `TestListJobsByTag_Pagination` | Medium | 5 tagged jobs, limit=2 → cursor returns next page |
| 13.2 | `TestListJobsByTag_MultipleTagsOnJob` | Easy | Job with 3 tags, search for one → found |
| 13.3 | `TestListJobsByTag_CrossProjectIsolation` | Medium | Same tag on jobs in different projects → only returns matching project |
| 13.4 | `TestListWorkflowsByTag_KeyOnly` | Easy | Search by key without value → returns all with that key |
| 13.5 | `TestListWorkflowsByTag_Pagination` | Medium | Multiple workflows, paginate |
| 13.6 | `TestListJobsByTag_SpecialCharsInTagValue` | Edge | Tag value with spaces, unicode, special chars |
| 13.7 | `TestListJobsByTag_EmptyTagValue` | Edge | Tag `{"key": ""}` (empty value) — search with value="" matches |

---

## 14. E2E Tests

### Current Coverage
- ✅ API key lifecycle (create, use, revoke)
- ✅ ListJobsByTag e2e

### Missing Tests

| # | Test | Difficulty | What it verifies |
|---|------|-----------|-----------------|
| 14.1 | `TestE2E_ScopeEnforcement` | Medium | Create key with `["jobs:read"]`, try POST job → 403, GET job → 200 |
| 14.2 | `TestE2E_EmptyScopesFullAccess` | Medium | Create key with `[]` scopes → all endpoints accessible |
| 14.3 | `TestE2E_JobVersionID` | Medium | Create job → response has version_id starting with "ver_", update → new version_id |
| 14.4 | `TestE2E_WorkflowVersionID` | Medium | Same for workflows |
| 14.5 | `TestE2E_JobCreatedBy` | Medium | Create job via internal secret with X-Actor-Id → created_by in response |
| 14.6 | `TestE2E_RolesLifecycle` | Hard | Create role → list → get → update → assign member → list members → remove → delete |
| 14.7 | `TestE2E_TagFilteringWorkflows` | Medium | Create workflows with tags → filter by tag key/value |
| 14.8 | `TestE2E_VersionPolicyDefault` | Easy | Create job without version_policy → response has "pin" |
| 14.9 | `TestE2E_DeleteJobWithActiveRuns` | Hard | Create job, trigger, try delete → 400/409 error |
| 14.10 | `TestE2E_UpdateJobVersionIncrement` | Easy | Create → update → update → verify version=3 |

---

## 15. Existing Tests That Need Verification

These tests exist but may not be testing what we think.

| # | Test | File | Issue |
|---|------|------|-------|
| 15.1 | `TestHandleRemoveMember_InvalidatesCache` | rbac_handler_test.go | Uses internal secret auth which sets projectID="" — cache key mismatch. Test passes vacuously. **Fix: use API key auth or directly verify correct cache key.** |
| 15.2 | `TestInternalSecretAuth_SetsActorFromHeaders` | actor_test.go | Captures `capturedActor` but never uses it. The test verifies the syncer was called but doesn't verify the actor context was set on the request. **Fix: add a handler that reads and returns actorFromContext.** |
| 15.3 | `TestRequirePermission_User_CacheHit` | scope_test.go | Uses non-atomic `callCount` variable — technically a race condition under `-race` since the handler runs synchronously. Actually safe since it's serial, but could be confusing. |
| 15.4 | `TestHandleAssignMember` | rbac_handler_test.go | Doesn't verify `granted_by` in response is set from actor context. Test uses internal secret auth (no actor) so granted_by would be empty. |
| 15.5 | `TestE2E_APIKeyLifecycle` | e2e_test.go | Only tests `stats:read` scope — doesn't verify scope *enforcement* (that a key WITHOUT a scope gets 403). |

---

## Execution Plan

### Batch 1: Easy tests (30 min, ~35 tests)
All items marked Easy from sections 1, 2, 3, 4, 6, 7, 9, 10, 11, 12, 13.
These are straightforward assertions with no setup complexity.

### Batch 2: Medium tests (1 hour, ~25 tests)
Cache concurrency, middleware chaining, snapshot verification, pagination,
cross-project isolation, E2E lifecycle.

### Batch 3: Hard/Edge tests (45 min, ~12 tests)
FK cascade verification, active run blocking, concurrent nanoid generation,
full RBAC lifecycle E2E, special characters in tags.

### Batch 4: Fix existing tests (20 min, ~5 tests)
Items from section 15 — fix tests that don't actually test what they claim.

### Total: ~77 new tests + 5 fixed tests ≈ 2.5–3 hours

---

## Priority Order

1. **Section 15** (fix broken/weak tests first — they give false confidence)
2. **Section 6** (RBAC handler error paths — most user-facing, highest risk)
3. **Section 8** (RBAC store — cascades, FK errors, edge cases)
4. **Section 10** (jobs with new fields — snapshot verification, delete safety)
5. **Section 5** (middleware edge cases)
6. **Section 12** (job versions — new columns)
7. **Section 14** (E2E — integration confidence)
8. **Sections 1, 2, 3, 4** (domain layer — low risk but good hygiene)
9. **Sections 9, 11, 13** (remaining store/query tests)
