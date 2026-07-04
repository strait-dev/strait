# apps/docs

Mintlify documentation site for Strait (docs.strait.dev).

## Purpose

The source of truth is the `.mdx` files in this directory plus the navigation tree in `docs.json`. A page that exists on disk but is not listed in `docs.json` is an orphan and fails the linter.

## Usage

```bash
bun run --cwd apps/docs dev
```

This runs `mintlify dev` on port 3001 for a local preview with hot reload.

`build` is a deliberate no-op (`echo 'Mintlify builds are handled by the Mintlify platform'`) because Mintlify's hosted platform builds and deploys the site directly from the repository; there is no local build artifact to produce. `typecheck` is a no-op for the same reason: there is nothing to type-check in a Mintlify docs project.

## Scripts

| Script | What it does | When to run it |
|---|---|---|
| `generate:api-fields` | Runs `scripts/generate-api-field-tables.mjs` to regenerate `api-reference/generated-fields.mdx` from the Huma route and operation definitions in `apps/strait`. | After adding or changing an HTTP route, field, or operation in `apps/strait/internal/api`. |
| `check:api-fields` | Runs the same generator with `--check` and fails if the checked-in `generated-fields.mdx` is out of date. | In CI, or locally before committing API changes. |
| `coverage` | Runs `scripts/report-docs-coverage.mjs`, which cross-references the Huma route registry against `api-reference`, `concepts`, and `guides` to report undocumented operations. | When auditing doc coverage after a batch of API changes. |
| `lint` | Runs `scripts/lint-docs.mjs`, the docs consistency linter. Enforces frontmatter (`title` and `description` on every page), no em-dash or en-dash characters (house style uses ASCII hyphens), every opening code fence has a language tag, internal links and anchors resolve, example hosts are normalized, and no orphan pages (every `.mdx` file is referenced in `docs.json`). It also checks several other invariants against the Go source, listed in the script's own header comment. | Before every commit that touches `apps/docs`; CI enforces it via `.github/workflows/docs.yml`. |
| `typecheck` | No-op (`echo 'No typecheck needed for Mintlify docs'`). | Never needs to be run; kept only so the script exists for turbo/CI tooling that expects it. |

## Structure

| Directory | Contents |
|---|---|
| `api-reference/` | REST API reference pages, including the generated `generated-fields.mdx` |
| `billing/` | Pricing and billing FAQ pages |
| `cli/` | Strait CLI reference (overview, API keys) |
| `compare/` | Comparisons against message queues, cron, workflow engines, and when not to use Strait |
| `concepts/` | Core platform concepts: jobs, runs, workflows, retries, webhooks, and related |
| `configuration/` | Environment variable reference |
| `development/` | Internal engineering notes such as release checklists and performance write-ups |
| `examples/` | Runnable snippets and request payload examples used by the linter's fixture checks |
| `guides/` | Task-oriented how-to guides (authentication, self-host backup/restore, troubleshooting, and related) |
| `images/` | Static images referenced by pages and `docs.json` metadata |
| `integrations/` | Third-party integration pages |
| `logo/` | Site logo assets referenced by `docs.json` |
| `operations/` | Incident-response and operational runbooks |
| `sdks/` | SDK reference and quickstart pages per language |
| `scripts/` | The generator and checker scripts described above |
| `tutorials/` | Longer end-to-end tutorials |
| `use-cases/` | Use-case-focused landing pages |

## Contributing

Every new `.mdx` page must be added to a `pages` array in `docs.json`, or `bun run --cwd apps/docs lint` flags it as an orphan page. Every page needs a frontmatter block with at least `title` and `description`.

For the full contributor workflow (validation commands, commit conventions, engineering rules), see the root [CLAUDE.md](../../CLAUDE.md) / [AGENTS.md](../../AGENTS.md) contributor guide rather than duplicating it here.

## Validation

```bash
bun run --cwd apps/docs lint
```
