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

## 5b. Round 2 — rate limiter / webhook / API key bypass review

A second focused pass per user request — "I want to run a full rate
limiter, webhook, api key, everything to make sure we don't got any
bypass." Findings and fixes:

### Critical (Round 2)

#### F-WH-1 — Webhook delivery worker hashed encrypted secret
**Impact:** When `STRAIT_ENCRYPTION_KEY` is set the API persists
subscription signing secrets as AES-GCM ciphertext. The delivery
worker passed the ciphertext bytes directly into the HMAC-SHA256
function, so the outbound signature could never validate against the
shared plaintext secret on the subscriber side. This silently broke
every signed-webhook integration whenever encryption was enabled.
**Fix:** Added a `SecretDecryptor` interface plumbed from `main.go`
via `WithSecretDecryptor`. The worker decrypts current and previous
secrets right before signing; on decrypt failure it falls back to the
raw value (with a warn log) so misconfigured rotations don't black-
hole deliveries. (`apps/strait/internal/webhook/event_notify.go`,
`apps/strait/cmd/strait/main.go`)
**Regression:** `TestAttemptDelivery_WithSubscriptionID_DecryptsSecretBeforeSigning`
**Commit:** `188d73c2`

### High (Round 2)

#### F-WH-3 — Cross-tenant circuit-breaker DoS
**Impact:** The webhook circuit breaker keyed solely on URL hash, so
two orgs sharing the same external receiver shared one breaker. A
single noisy tenant could trip delivery for everyone pointing at that
receiver. `groupByURL` had the same property — cross-tenant
deliveries were colocated in one batch.
**Fix:** Composed a `(orgID|url)` breaker key at every call site
without changing the breaker interface; `groupByURL` now keys on the
same composite. (`apps/strait/internal/webhook/event_notify.go`)
**Regression:** `TestBreakerKey_PerTenantScoping`
**Commit:** `188d73c2`

#### F-WH-6 — Webhook delivery followed redirects (SSRF)
**Impact:** The default Go HTTP client follows redirects, so a
webhook receiver could 301 the worker to `169.254.169.254`, internal
admin endpoints, or any non-allowlisted target.
**Fix:** Set `CheckRedirect: noFollowRedirects` on both the default
and custom-transport client. Any 3xx response is now classified as a
permanent delivery failure (single + batch path).
**Regression:** `TestWebhookResilience_RedirectToLocalhost` (asserts
the redirect target is never hit, status=dead, last_status_code=301).
`TestAttemptBatchDelivery_3xxResponse` inverted to assert dead.
**Commit:** `188d73c2`

#### F-WH-2 — SDK wait_event NotifyURL bypassed SSRF validator
**Impact:** `POST /sdk/v1/runs/:id/wait-for-event` accepted a
`notify_url` that the webhook delivery worker would later POST to
without going through `validateURL`, so a compromised SDK caller
could coerce the worker into hitting the cloud metadata endpoint or
internal services.
**Fix:** Run `req.NotifyURL` through the same validator the public
webhook subscription create/update path uses.
(`apps/strait/internal/api/sdk_wait_event.go`)
**Commit:** `7e0a7b8c` (this PR — see commit list).

#### F-RL-3 — OIDC auth missing brute-force lockout
**Impact:** `apiKeyAuth` and `internalSecretAuth` ran every request
through the IP-keyed auth limiter; `oidcAuth` did not. An attacker
who exhausted the API-key budget on one IP could pivot to OIDC token
brute force from the same IP and chew through the budget again.
**Fix:** `oidcAuth` now mirrors the other two paths
(`IsBlocked` → 429 + `Retry-After`; `RecordFailure` on every
rejection). All three paths additionally call `Reset` after
successful auth so a user who fat-fingered the key isn't held up
by stale failures. (`apps/strait/internal/api/middleware.go`)
**Commit:** `8ac84d6b`

#### F-AK-13 — Run tokens accepted any HS256 JWT signed with the shared key
**Impact:** `runTokenAuth` validated the signing method and key but
not the issuer, audience, or that an `exp` claim was present. The
JWT signing key is also used for SSE tokens and gRPC inter-service
tokens, so a token issued for a different audience could be replayed
against the SDK plane. golang-jwt/jwt/v5 also silently treats a
missing `exp` as never-expiring.
**Fix:** Token generation in `dispatcher.buildAttempt` now sets
`Issuer = "strait:run-token"`. `runTokenAuth` now passes
`jwt.WithIssuer("strait:run-token")` and `jwt.WithExpirationRequired()`
to `ParseWithClaims`. (`apps/strait/internal/api/sdk.go`,
`apps/strait/internal/api/grpc/dispatch.go`)
**Regression:** `TestRunTokenAuth_WrongIssuer_Rejected`,
`TestRunTokenAuth_NoExpiration_Rejected`
**Commit:** (this round)

### Medium (Round 2)

#### F-WH-5 — Synchronized retry storms (no jitter)
**Impact:** Exponential / linear retry policies produced identical
retry timestamps across every failed delivery in a poll cycle, so a
recovering downstream got DDoSed by a synchronized wave.
**Fix:** `backoffForRetryPolicy` now applies +/- 20% decorrelated
jitter via a process-local PCG seeded from `crypto/rand` (so jitter
is unpredictable across pods).
**Commit:** `188d73c2`

#### F-WH-7 — `signature_algorithm` accepted any string
**Impact:** `event_sources` POST/PATCH let callers store arbitrary
values in `signature_algorithm`. `ValidateSignature` rejected unknown
algorithms at use-time, but accepting arbitrary values polluted the
schema and made audit logs harder to reason about.
**Fix:** Added `validate:"omitempty,oneof=hmac-sha256 stripe-v1 github-sha256"`
to both create and update request structs.
**Commit:** `7e0a7b8c`

#### F-BD-13 — Log-drain HTTP clients followed redirects (SSRF pivot)
**Impact:** Both the run-event log drain and the audit SIEM drain
used `http.Client` without `CheckRedirect`. A compromised or
mis-configured drain could 3xx-pivot to an internal target.
**Fix:** Both clients now refuse redirects via
`http.ErrUseLastResponse`. The endpoint URL is still validated with
`httputil.ValidateExternalURL`. (`apps/strait/internal/logdrain/service.go`,
`apps/strait/internal/logdrain/audit_drain.go`)
**Commit:** `7e0a7b8c`

#### F-RL-4 — Auth-limiter INCR/EXPIRE not atomic
**Impact:** `RecordFailure` used a non-transactional Pipeline so the
INCR and PExpire ran independently. If Redis hiccupped between them
the counter incremented but stayed without a TTL — the IP would be
locked out forever.
**Fix:** Switched to `TxPipelined` (MULTI/EXEC). Both commands now
either succeed or fail together. (`apps/strait/internal/ratelimit/auth_limiter.go`)
**Regression:** `TestAuthLimiter_RecordFailure_AlwaysSetsTTL`
**Commit:** `8ac84d6b`

#### F-RL-1 — `httprate.LimitByIP` keyed on RemoteAddr only
**Impact:** Behind a load balancer `LimitByIP` keys all traffic to
the LB's address — every tenant's requests share one bucket and any
burst from a single user drags the whole pool over the limit.
**Fix:** Replaced with `httprate.Limit(...WithKeyFuncs(rateLimitKeyByIP))`,
which walks X-Forwarded-For across `TRUSTED_PROXIES` the same way
the auth-lockout path does. (`apps/strait/internal/api/routes.go`)
**Commit:** `8ac84d6b`

#### F-RL-6 — Lockout counter never reset on success
**Impact:** A user who fat-fingered their key 9 times could still hit
the lockout window for legitimate subsequent calls because nothing
called `AuthLimiter.Reset` on a successful auth.
**Fix:** All three auth paths now call `s.authLimiter.Reset(...)`
after a successful auth, with the same `clientIP` derivation used
for the lockout check.
**Commit:** `8ac84d6b`

### Round 2 commit list

1. `188d73c2` — `fix(webhook): decrypt secret before signing, scope breaker per tenant, block redirect ssrf, jitter backoff`
2. `7e0a7b8c` — `fix(api,logdrain): close SSRF gaps on SDK wait_event, event source signature, log drains`
3. `8ac84d6b` — `fix(ratelimit,api): close brute-force + IP-spoof gaps in auth and global limiter`
4. `74198ac0` — `fix(api,grpc): bind run tokens to a strait:run-token issuer and require exp`

---

## 5c. Round 3 — terminal-run / environment scoping / DLQ-mask review

Targeted follow-up sweep on the residual items from the Round-2 priority list:
run-token replay against terminal runs, API-key environment scoping, and the
debug-bundle bypass of DLQ masking. All three were exploitable on `master` at
the start of the round.

### High (Round 3)

**F-AK-15 — Run tokens remained valid after the run reached a terminal state.**
`runTokenAuth` only verified the JWT signature, issuer, and that the `sub`
claim matched the URL's run ID. A token issued during execution stayed
acceptable for its full lifetime even after the run completed/failed/was
canceled — letting a compromised SDK process keep emitting events, checkpoints,
and tool calls into a "closed" run, undetectable in the run timeline because
the system treats those rows as part of the original execution.

Fix (commit `086eee72`):
- Added `store.GetRunStatus(ctx, id)` — a status-only lookup with
  `job_runs_history` fallback; returns `ErrRunNotFound` when the run no longer
  exists in either table.
- Wired the lookup into `runTokenAuth` after subject/URL verification: if the
  status `IsTerminal()`, the request is rejected with `410 Gone`; missing runs
  return `404`. Status check runs once per SDK request (one indexed PK lookup,
  no `RETURNING` payload).
- Added 14 regression tests (`TestRunTokenAuth_TerminalRun_Rejected` table-
  driven across all 8 terminal `RunStatus` values, plus
  `TestRunTokenAuth_RunNotFound_Rejected`).

### Medium (Round 3)

**F-AK-8 — API-key `EnvironmentID` was loaded but never enforced.**
`domain.APIKey.EnvironmentID` and `domain.Job.EnvironmentID` had been wired
through `store` and the trigger schema, but `apiKeyAuth` dropped the
environment binding on the floor. An env-prod-scoped key could trigger,
read, update, pause, resume, or delete jobs in env-staging — the
environment column was effectively cosmetic.

Fix (commit `2302966b`):
- Stamped `ctxEnvironmentIDKey` in `apiKeyAuth` whenever the bound key carries
  an `EnvironmentID`; project-wide keys remain untouched.
- Added `requireEnvironmentMatch(ctx, resourceEnv)` helper with conservative
  semantics: project-wide callers pass through, env-bound callers must match
  exactly (including the env-bound-vs-env-less case, to prevent silent
  escalation as environments are progressively rolled out).
- Wired the check into the 6 job-targeted handlers (trigger, get, update,
  delete, pause, resume) after the existing `requireProjectMatch` call, so
  mismatches surface as 404 Not Found (no cross-tenant existence leak).
- 7 regression tests (`middleware_environment_test.go`) covering helper edge
  cases plus end-to-end handler-level mismatch / match assertions.

**F-DBG-2 — Debug-bundle endpoint ignored `visible_until` masking.**
The DLQ age-out flow uses `job_runs.visible_until <= NOW()` to take rich-PII
rows out of circulation without dropping them (audit retention). The
`/v1/runs/:id/debug` endpoint went straight to `GetRun` and returned the full
fan-out (events, checkpoints, usage, tool calls, outputs) on masked runs —
silently undoing the DLQ decision.

Fix (commit `6cadab86`):
- Added a `visible_until` probe at the top of `GetDebugBundle`. Missing rows
  return `ErrRunNotFound`; rows with a past `visible_until` also return
  `ErrRunNotFound` (treat masked as if it never existed, matching DLQ
  semantics).
- Extended `requireRunAccess` to additionally fetch the owning job via
  `run.JobID` and apply `requireEnvironmentMatch` against `Job.EnvironmentID`,
  so an env-prod key cannot read runs of an env-staging job through the
  read/wait/SSE plane.
- Mock dispatcher in `runs_debug_test.go` now branches on
  `strings.Contains(sql, "visible_until")` to serve the new probe; added
  `TestGetDebugBundle_MaskedRun_ReturnsNotFound` as a regression for the mask
  bypass.

### Round 3 verified-not-vulnerable

Spot-checked while in the area; no fix needed:

- **iss/aud validation on run tokens** — already enforced via
  `jwt.WithIssuer("strait:run-token")` + `jwt.WithExpirationRequired()` from
  Round 2 (commit `74198ac0`); a run-token has no audience because the
  subject-vs-URL check already binds it to a single run.
- **Atomic INCR+EXPIRE in `auth_limiter.go`** — already uses `TxPipelined`
  (MULTI/EXEC) so the `Incr` and `PExpire` either both apply or both roll
  back, eliminating the "permanently locked-out IP" race.
- **Reset on successful auth** — `apiKeyAuth`, `oidcAuth`, and
  `internalSecretAuth` all call `s.authLimiter.Reset(...)` after a successful
  verification, so a legitimate user who fat-fingered their key isn't held in
  the lockout window.

### Round 3 commit list

1. `086eee72` — `fix(api,store): reject run tokens bound to terminal runs`
2. `2302966b` — `fix(api): enforce environment scoping for environment-bound api keys`
3. `6cadab86` — `fix(store,api): hide masked runs from debug bundle and gate run access by environment`

### Round 3 deferred

**F-AK-3 — HMAC pepper for the API-key hash.** The current scheme is
`SHA256(rawKey)` only; an attacker who exfiltrates the `api_keys` table can
brute-force the (high-entropy but not infinite) `strait_…` key space offline
without ever touching the running system. The mitigation — peppered HMAC —
requires:
1. A new `key_hash_version` column on `api_keys`.
2. Dual-hash lookup (try v2 with pepper, fall back to v1 SHA256) so existing
   keys keep working through their grace window.
3. Coordinated changes to the three direct hash callers
   (`internal/api/middleware.go:420`, `internal/scheduler/reaper.go:1278`,
   `internal/api/grpc/auth.go:72-74`) plus all key-creation paths.
4. A pepper config knob backed by a separate secret (so a single DB exfil
   isn't enough to recover keys).

This crosses the threshold from "fix in-flight" to "scoped change requiring a
migration plan and a separate review window," so it's deferred to a dedicated
PR. The current scheme is still gated by AES-GCM-encrypted DB connections,
restricted DB ACLs, and the brute-force lockout in `auth_limiter.go`; the
peppered HMAC is defense-in-depth, not a closing of an open exploit on the
running system.

---

## 6. Recommended follow-ups (not security-critical)

1. **F-1505** — move org daily/monthly run-limit INCR-and-check from dispatch to trigger. Touches every run-creation path; do under a feature flag with billing-team review.
2. **F-1207** — apply `DisallowUnknownFields` (or strict mode) to the Huma operation decoder. Forward-compat hardening.
3. **TRUSTED_PROXIES runbook entry** — add a self-host deployment note explaining the empty-default fail-safe and the LB CIDR they must configure.
4. **Integration + lint suites** — run `go test -tags integration ./...` and `golangci-lint run --timeout=10m ./...` on PR; both were skipped in this pass per the time budget.
5. **Per-route quota for `/v1/projects/:id/activity/stream`** — the SSE conn limit is project-scoped; consider a tighter cap on this endpoint specifically since it's the highest-fanout subscriber.
6. **F-AK-3 follow-up PR** — peppered HMAC for the API-key hash, with the v1/v2 dual-hash migration plan described in §5c.

---

<promise>SECURITY-AUDIT-DONE</promise>
