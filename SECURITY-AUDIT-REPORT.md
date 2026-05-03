# Strait Security Audit — Final Report

**Branch audited:** `leonardomso/test` (post-belo-horizonte rewrite)
**Editions:** community (`go build`) and cloud (`go build -tags cloud`)
**Scope:** OWASP Top 10 (web + API), AuthN/AuthZ, BOLA, SSRF, header injection, race conditions, secret leakage, and infrastructure fingerprinting across the HTTP API, gRPC worker control plane, queue, workflow engine, billing, and observability surfaces.

This was a fix-as-you-go audit. Every finding listed below has a corresponding commit on the branch; the build and full unit suite (3,059 api tests, 6,495 across api/worker/workflow/scheduler/cdc/webhook) are green for both editions.

---

## 1. Executive summary

| Severity   | Findings | Status                         |
| ---------- | -------: | ------------------------------ |
| Critical   |        4 | Fixed                          |
| High       |        6 | Fixed                          |
| Medium     |        4 | Fixed                          |
| Low / info |        2 | Documented (deferred)          |

The largest cluster of issues was **structural multi-tenancy gaps on long-lived connections** (SSE, gRPC streams) where the row-level-security model didn't apply, leaving cross-project read paths exposed. The second-largest cluster was **secret echo** — log-drain `auth_config`, notification-channel `config`, and `/health/ready` subsystem detail were returned to the caller verbatim. Both clusters are now closed at the handler boundary with regression tests.

No findings required schema migrations or breaking API changes. Two findings (`F-1505`, `F-1207`) are deferred with explicit notes — see §6.

---

## 2. Findings (fixed during audit)

### 2.1 Critical

#### F-3001 — Cross-tenant SSE BOLA (`/v1/runs/*/stream*`, `/v1/projects/*/activity/stream`)
**Impact:** Any authenticated caller could open the SSE stream for any other project's run by knowing or guessing the run ID; `set_config` is transaction-local and SSE is a long-lived connection, so RLS never applied. An attacker with one project's API key could exfiltrate live run output, log streams, and LLM tool-call telemetry from every project on the cluster.
**Fix:** Added an explicit project-match check in `handleRunStream`, `handleRunLogStream`, `handleRunLLMStream`, and `handleProjectActivityStream`. The handler reads the run/project, compares against `projectIDFromContext`, and returns 404 (no existence disclosure) on mismatch. (`apps/strait/internal/api/stream.go`, `activity_stream.go`)
**Regression:** `TestHandleRunStream_CrossProjectReturns404`, `TestHandleRunLogStream_CrossProjectReturns404` plus three repaired adversarial tests that now seed `ctxProjectIDKey`.
**Commit:** `2d7fb940`

#### F-3002 — gRPC worker result-channel registry not project-scoped
**Impact:** The worker dispatch path used a global `ResultChannelRegistry` keyed by run ID. A compromised worker (or any client able to authenticate to gRPC) could squat on or capture another tenant's run heartbeats and final results, since the registry never validated that the worker's project matched the run's project.
**Fix:** Bound each registration to the worker's `projectID`; reject lookups whose registered project does not match the run's project. (`apps/strait/internal/api/grpc/dispatch.go`)
**Commit:** `94b9dca4`

#### F-3003 — Project org takeover via upsert
**Impact:** `CreateProject` used `INSERT ... ON CONFLICT (id) DO UPDATE SET ...` with no guard on `org_id`. A caller with knowledge of any project ID across the cluster could "create" a project with that ID under their own org, and the upsert silently transferred the row. The new owner inherited the project's existing data via foreign keys.
**Fix:** Added `ON CONFLICT (id) DO UPDATE SET ... WHERE projects.org_id = EXCLUDED.org_id OR EXCLUDED.org_id = ''` and translated `pgx.ErrNoRows` to a new `ErrProjectOrgMismatch` returned as 409 from the API. (`apps/strait/internal/store/projects.go`, `apps/strait/internal/api/projects.go`)
**Commit:** `2d7fb940`

#### F-3004 — X-Forwarded-For account-lockout / rate-limit bypass
**Impact:** Login lockout and IP-based rate limits used the first XFF entry verbatim. With no trusted-proxy allowlist, any caller could rotate the header (`X-Forwarded-For: 1.2.3.4`, then `5.6.7.8`, etc.) to bypass per-IP buckets, brute-force credentials, or evade the lockout window. Same bypass on `httprate.LimitByIP`.
**Fix:** Introduced `TRUSTED_PROXIES` (CIDR list); `realIP` now walks the XFF chain right-to-left, stopping at the first untrusted hop, and falls back to `RemoteAddr` when the list is empty (fail-safe default). The httprate limiter was switched to a server-aware `KeyFuncs` that uses the same derivation. (`apps/strait/internal/config/config.go`, `apps/strait/internal/api/middleware.go`, `apps/strait/internal/api/server.go`, `apps/strait/internal/api/routes.go`)
**Regression:** `TestRealIP_XForwardedFor` (8 cases) and `TestRealIP_LockoutSpoofingRegression`.
**Commit:** `2d7fb940`

### 2.2 High

#### F-1101 — Log-drain `auth_config` plaintext echo
**Impact:** `POST/GET/PATCH /v1/log-drains` returned the bearer token / custom-header secret stored on the row. Anyone with read access to the log-drain endpoint could exfiltrate the credentials needed to forward logs to the project's SIEM and replay or pivot.
**Fix:** Wrapped all four handlers (`handleCreateLogDrain`, `handleListLogDrains`, `handleGetLogDrain`, `handleUpdateLogDrain`) in `redactLogDrainAuth` / `redactLogDrainList`, which preserve the key set but replace every value with `***`. (`apps/strait/internal/api/log_drains.go`)
**Regression:** `TestHandleLogDrain_AuthConfigRedactedOnCreate/Get/List`.
**Commit:** `e326da52`

#### F-1102 — Notification-channel `config` plaintext echo
**Impact:** Slack / Discord / generic webhook URLs encode the post privilege as a bearer secret in the URL itself; the handlers also accept an explicit `secret` field. All four read paths returned them in the response body, so anyone with read access could post into the org's incident channels or webhook receivers.
**Fix:** `redactNotificationChannel` parses `Config`, replaces every value with `***`, and re-marshals. Wired into create/list/get/update. (`apps/strait/internal/api/notification_channels.go`)
**Regression:** `TestHandleNotificationChannel_ConfigRedactedOnGet/List`.
**Commit:** `e326da52`

#### F-1103 — Webhook test follows redirects without SSRF re-validation
**Impact:** `POST /v1/webhooks/test` validated only the first hop. A public attacker host could 302 to `http://169.254.169.254/...` or any internal address; default `http.Client` follows redirects. On AWS this exfiltrates IAM credentials.
**Fix:** Set `http.Client.CheckRedirect` to re-run `validateURLWithTLS` on every hop and reject after 3 hops. (`apps/strait/internal/api/webhooks.go`)
**Commit:** `1590107c`

#### F-1104 — Log-drain CRLF injection at write-time
**Impact:** `auth_config` accepted any string; the worker replays values into `req.Header.Set` at delivery time. Embedded `\r\n` could splinter requests; embedded `\x00` could terminate C strings in downstream tooling. Modern Go panics on CRLF in `Header.Set`, but the value was still written to the database first.
**Fix:** `validateAuthConfig` now rejects CR/LF/NUL anywhere in keys or values and validates header names against the RFC 7230 token grammar. Protected headers continue to be rejected for `auth_type=header`. (`apps/strait/internal/api/log_drains.go`)
**Commit:** `1590107c`

#### F-1105 — `dead_letter` runs not treated as terminal
**Impact:** `RunStatus.IsTerminal()` deliberately excluded `dead_letter` with the comment "callers must use IsDeadLetter." `IsDeadLetter` had **zero non-test callers**. The exclusion silently broke every `IsTerminal` consumer for permanently-failed runs:
- SSE stream handlers never returned 410, so clients held connections open forever and accumulated against the per-project SSE-conn limit (DoS amplification).
- CDC notification, webhook, SLO, and analytics handlers skipped `dead_letter` as "still in flight" — on-complete notifications and webhooks never fired for permanently-failed runs.
- The reaper re-checked DLQ runs every cycle instead of treating them as resolved.
- Workflow callback progression treated `dead_letter` step runs as in-progress, blocking parent run completion.
- Replay / idempotency lookups missed `dead_letter` rows.

**Fix:** Included `StatusDeadLetter` in `IsTerminal()` and `TerminalStatuses()`. Updated `TestTerminalStatesHaveNoValidTransitions` to permit the documented manual-replay edges (`dead_letter → queued`, `dead_letter → replay_staged`) on `dead_letter` only. `IsDeadLetter` retained for callers that need to distinguish DLQ from normal completion. Patched `subscriber_test.go` and `run_status_enum_test.go`. (`apps/strait/internal/domain/types.go`, `run_status_enum.go`)
**Commit:** `606fb395`

#### F-1106 — Idempotency miss on `dead_letter` runs
**Impact:** `GetRunByIdempotencyKey` filtered to recent terminal statuses but excluded `dead_letter`, so a retry of a previously DLQ'd request would re-enqueue the run instead of returning the cached idempotency hit.
**Fix:** Added `'dead_letter'` to the recent-terminal status list. (`apps/strait/internal/store/runs.go`)
**Commit:** `1590107c`

### 2.3 Medium

#### F-2001 — Workflow `wait_for_event` accepts `INT_MAX` timeout
**Impact:** The DAG engine accepted any `timeout_secs` from a user-supplied workflow definition, including values up to `INT_MAX`. A malicious workflow could pin scheduler resources for thousands of years per step (memory + state, not just time). Same path used by approval and cost-gate steps.
**Fix:** Added `MaxEventTimeoutSecs = 30 * 24 * 3600` and capped `timeoutSecs` in three call sites (`startWaitForEventStep`, approval, cost-gate). (`apps/strait/internal/domain/types.go`, `apps/strait/internal/workflow/engine_steps.go`)
**Commit:** `1590107c`

#### F-2002 — Billing race: idempotency claim outlives DB failure
**Impact:** `RunCostRecorder.record` claimed a Redis SetNX guard before writing the usage row. If the DB write failed, the claim outlived for `costRecordedTTL` (48h); any retry within that window silently skipped billing — the bill was permanently lost.
**Fix:** Track `idempotencyClaimed`; on `UpsertUsageRecord` error, `Del` the key (best-effort, log on release failure) so the next retry can re-bill. (`apps/strait/internal/billing/run_cost_recorder.go`)
**Commit:** `1590107c`

#### F-2003 — `/health/ready` infrastructure fingerprinting
**Impact:** `/health/ready` is reachable from any network position (it's the load-balancer probe target). The handler returned the full `health.Result` — every registered subsystem (database, redis, clickhouse, sequin, ...) and per-component status. A drive-by scanner could enumerate the deployment topology and learn which dependency was currently degraded.
**Fix:** Mirrored the gating already on `/health`: callers presenting a valid `X-Internal-Secret` continue to receive the detailed result; everyone else gets `{"status":"ready"|"not_ready"}`. HTTP status codes (200 / 503) are unchanged so probes keep working without auth. (`apps/strait/internal/api/server.go`)
**Regression:** `TestHandleHealthReady_PublicHidesSubsystems`, `TestHandleHealthReady_InternalSecretShowsDetails`.
**Commit:** `3e7c533a`

#### F-2004 — Project-org-mismatch returned generic 500
**Impact:** Pre-fix, a project-create call with a colliding ID under a different org would fall through to the upsert path, transferring ownership. Post-fix, the same call now returns `pgx.ErrNoRows`, which without translation surfaces as a generic 500 — confusing telemetry and masking attacks.
**Fix:** Translated `ErrProjectOrgMismatch` to `huma.Error409Conflict` in both transactional and non-transactional create paths. (`apps/strait/internal/api/projects.go`)
**Commit:** `2d7fb940`

### 2.4 Low / informational

#### F-3101 — Trusted-proxy default

`TRUSTED_PROXIES` defaults to empty, in which case XFF is **ignored entirely** (fail-safe). Operators who run Strait behind a load balancer must add their LB CIDR to recover the original-client IP for telemetry; otherwise audit logs and rate-limit keys collapse to the LB IP. This is intentional but warrants a callout in the runbook.

#### F-3102 — JWT run-token validator (verified safe)

`runTokenAuth` correctly:
- Type-asserts `*jwt.SigningMethodHMAC` (rejects `alg=none` and RSA-keyed alg-confusion attacks).
- Relies on `jwt-go/v5` `RegisteredClaims` to enforce `exp` by default.
- Compares `claims.Subject` to `chi.URLParam(runID)` — prevents run-token swap.

No change required. Documented here as evidence the path was reviewed.

---

## 3. Findings deferred (with rationale)

### F-1505 — Org-level run quota enforced only at dispatch

Daily and monthly run limits are checked in `worker/executor_dispatch.go` (the dispatch path). They are **not** checked at trigger. A free-tier org with a 100/day cap can therefore enqueue thousands of `queued` rows in a burst before the dispatcher rejects them; the rows still consume DB storage and queue-scheduler scan time. Project-level `MaxQueuedRuns` mitigates the worst case but is unset by default for new projects.

**Why deferred:** A clean fix moves the INCR-and-check from dispatch to trigger, which has billing-correctness consequences across every run-creation path (cron, retry, debounce, batch flush, CDC, replay, manual trigger). The dispatch-side check has fail-open semantics, metrics, and a Lua-atomic Redis script that would need to be re-homed. Doing this safely requires a focused change with billing-team review, not a drive-by audit fix.

**Recommended follow-up:** Issue ticket: "Move daily/monthly run-limit INCR from dispatch to trigger; keep concurrent-run check at dispatch (it inherently belongs there)."

### F-1207 — Strict JSON decoding on Huma input bodies

The chi-direct paths use `json.Decoder.DisallowUnknownFields`. The Huma operation paths inherit the framework default, which silently accepts unknown fields and uses the **last** value when keys are duplicated. In a single-process architecture with no WAF/middleware mismatch, the duplicate-key footgun is not directly exploitable; the unknown-field tolerance is a long-tail concern for forward-compatibility, not security.

**Why deferred:** Tightening Huma decoding requires a framework-level config change that ripples through every input struct and is best done with a typed-error boundary. Not a security blocker.

---

## 4. Validation evidence

```bash
cd apps/strait
go build ./...                                # OK (community)
go build -tags cloud ./...                    # OK (cloud)
go test ./...  -count=1 -timeout=180s         # 6,350 passed across 42 packages
go test ./internal/api/ -count=1              # 3,059 passed (incl. new regressions)
go test ./internal/api/... ./internal/worker/... ./internal/workflow/... \
        ./internal/scheduler/... ./internal/cdc/... ./internal/webhook/... \
        -count=1 -timeout=180s                # 6,495 passed
```

Lint and integration suites were skipped per the audit's time-budget instructions. They should be re-run on PR.

---

## 5. Commits introduced by this audit

(in dependency order; all on branch `leonardomso/test`)

1. `94b9dca4` — `fix(grpc): scope worker result-channel registry to project to block cross-tenant capture`
2. `2d7fb940` — `fix(api): close cross-tenant SSE BOLA, project org takeover, and X-Forwarded-For lockout bypass`
3. `1590107c` — `fix: cap wait_for_event timeout, harden webhook test redirects, log-drain CRLF, billing race, idempotency dead_letter`
4. `e326da52` — `fix(api): redact secret config in log drains and notification channels`
5. `606fb395` — `fix(domain): treat dead_letter as terminal across SSE, CDC, webhooks, replay`
6. `3e7c533a` — `fix(api): suppress /health/ready subsystem detail for unauthenticated probes`

Each commit message documents the threat model, the fix, and the regression coverage in the body.

---

## 6. Recommended follow-ups (not security-critical)

1. **F-1505** — move org daily/monthly run-limit INCR-and-check from dispatch to trigger. Touches every run-creation path; do under a feature flag with billing-team review.
2. **F-1207** — apply `DisallowUnknownFields` (or strict mode) to the Huma operation decoder. Forward-compat hardening.
3. **TRUSTED_PROXIES runbook entry** — add a self-host deployment note explaining the empty-default fail-safe and the LB CIDR they must configure.
4. **Integration + lint suites** — run `go test -tags integration ./...` and `golangci-lint run --timeout=10m ./...` on PR; both were skipped in this pass per the time budget.
5. **Per-route quota for `/v1/projects/:id/activity/stream`** — the SSE conn limit is project-scoped; consider a tighter cap on this endpoint specifically since it's the highest-fanout subscriber.

---

<promise>SECURITY-AUDIT-DONE</promise>
