# Strait Unified CLI (`apps/cli`)

This document freezes the architecture and UX contract for the unified TypeScript CLI.

Status: **Phase G implementation complete, Phase H cleanup/cutover active**

## Goals

- One canonical CLI: `strait`
- TypeScript implementation with `stricli` command modeling
- End-to-end Effect runtime architecture
- Hybrid interaction model (interactive only where it improves flow)
- Script-safe deterministic output for automation
- Code-first project flow centered on `strait.config.ts`

## Non-goals (for this phase)

- Implementing all migrated commands
- Backend deployment/runtime changes
- Hosted runtime execution

---

## Command parity matrix (Go CLI -> unified JS CLI)

Legend:
- **Keep**: feature remains in unified CLI
- **Rework**: feature remains but command UX/shape will be modernized
- **Drop**: removed from unified CLI
- **Phase**: planned migration phase from the approved roadmap

| Existing Go Surface | Unified JS Surface | Decision | Phase |
| --- | --- | --- | --- |
| `auth`, `login`, `logout` | `auth login`, `auth logout`, `auth whoami` | Rework | B |
| `context` (`create/use/list/current`) | `context create/use/list/current` | Keep | B |
| `jobs` | `jobs ...` | Keep | C |
| `runs` | `runs ...` | Keep | C |
| `workflows` | `workflows ...` | Keep | C |
| `workflow-runs` | `workflow-runs ...` | Keep | C |
| `events` | `events ...` | Keep | C |
| `trigger`, `triggers`, `send`, `listen` | `events/trigger` family (final grouping TBD in implementation) | Rework | C |
| `secrets` | `secrets ...` | Keep | C |
| `api-keys` | `api-keys ...` | Keep | C |
| `stats`, `health` | `stats`, `health` | Keep | B/C |
| `init` | `init` (interactive-first) | Rework | B |
| `dev` | `dev` (interactive-first + deterministic non-TTY) | Rework | D |
| `build` (new) | `build --dry-run` | New | D |
| `deploy` (new) | `deploy [--env] [--dry-run]` | New | G |
| `promote` (new) | `promote <deploymentVersionId> --env <env>` | New | G |
| `rollback` (new) | `rollback --to <deploymentVersionId> --env <env>` | New | G |
| `validate`, `apply`, `diff`, YAML declarative flow | **Removed from unified CLI** | Drop | H |
| `export` YAML manifests | Re-evaluate as code export/import tools | Rework | H |
| `tui`, `top`, `logs`, `trace`, `wait` | Retain where operationally justified; script-safe defaults | Rework | C/G |
| `server`, `serve`, `migrate`, `db`, `backup`, `profile` | Admin/ops namespace in unified CLI (`server ...`) | Rework | C/G |
| `docs`, `completion`, `version`, `upgrade`, `check`, `diagnose`, `cleanup`, `fixtures`, `extension`, `alias`, `run`, raw `api` | Keep selectively with script-first behavior | Rework | C/G/H |

### Migration boundary

The unified CLI is code-first. YAML declarative commands (`validate/apply/diff`) are removed; deployment lifecycle now flows through `build -> deploy -> promote/rollback`.

---

## Interaction matrix (frozen policy)

| Command group | TTY (default) | Non-TTY / pipe | `--ci` |
| --- | --- | --- | --- |
| `init` | Interactive prompts + guided output | Non-interactive validation + explicit errors | Non-interactive |
| `deploy` | Interactive confirmations/progress | Deterministic plain/json output | Deterministic, no prompts |
| `dev` | Interactive status/progress | Deterministic status lines | Deterministic, no prompts |
| Query/list/get (`jobs list`, `runs get`, etc.) | Script-first deterministic output | Script-first deterministic output | Script-first deterministic output |
| Admin/ops (`migrate`, `server`, `health`) | Script-first deterministic output | Script-first deterministic output | Script-first deterministic output |

Rules:
1. Interactive mode is opt-in by command design, not global.
2. Non-TTY always disables prompts and animation.
3. `--ci` forces non-interactive deterministic mode even on TTY.
4. JSON output remains stable and machine-consumable.

---

## Animation and accessibility policy

Animation is allowed only as micro-feedback:
- Spinner/progress indicators
- Short transition states that never block completion

Animation must be disabled when any of the following is true:
- output is not a TTY
- `--ci` is enabled
- user prefers reduced motion (`NO_COLOR`, terminal settings, or OS preference where detectable)

Fallback requirements:
- every animated state must have a plain text equivalent
- completion and errors must be visible without animation
- no ANSI-dependent information hierarchy

---

## Runtime architecture (Effect + stricli)

The CLI runtime is layered as Effect services:

1. **Config service**: resolve `strait.config.ts`, workspace dirs, environment overrides
2. **Auth service**: credentials/profile/context resolution and persistence
3. **API service**: typed HTTP client, retries, response/error mapping
4. **Filesystem/process service**: local build, file discovery, subprocess orchestration
5. **Renderer service**: interactive vs deterministic renderers selected per command mode
6. **Telemetry/log service**: command diagnostics and structured debug output

`stricli` command handlers compose these services and return domain-level results; rendering is separate from business logic.

---

## TSDoc + readability standards

Required:
- TSDoc on every exported symbol in `apps/cli/src/**`
- TSDoc on Effect service interfaces and constructors
- Error types document retryability and operator action

Readability rules:
- one function = one level of abstraction
- command handlers orchestrate; services implement mechanics
- no hidden global mutable state
- no dual canonical paths for the same concept

---

## Deliverables for this phase

- Architecture and UX policy captured in this file
- Parity matrix established for Go -> JS migration
- Interaction and animation rules frozen for implementation
