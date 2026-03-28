# SDK/CLI/MCP Docs Sync

Documentation from external SDK repos is automatically synced into this directory.

## How it works

1. Each SDK repo has a `docs/` directory with Mintlify-compatible MDX files
2. The `sync-docs` GitHub Action in this repo clones each SDK repo and copies `docs/*.mdx`
3. The action runs every 6 hours, on manual trigger, or when an SDK repo dispatches an event

## Synced directories

| Source repo | Target directory | Docs URL |
|-------------|-----------------|----------|
| `strait-dev/strait-ts-sdk` | `apps/docs/sdks/typescript/` | `docs.strait.dev/sdks/typescript/*` |
| `strait-dev/strait-python-sdk` | `apps/docs/sdks/python/` | `docs.strait.dev/sdks/python/*` |
| `strait-dev/strait-go-sdk` | `apps/docs/sdks/go/` | `docs.strait.dev/sdks/go/*` |
| `strait-dev/strait-ruby-sdk` | `apps/docs/sdks/ruby/` | `docs.strait.dev/sdks/ruby/*` |
| `strait-dev/strait-rust-sdk` | `apps/docs/sdks/rust/` | `docs.strait.dev/sdks/rust/*` |
| `strait-dev/strait-cli` | `apps/docs/cli/` | `docs.strait.dev/cli/*` |
| `strait-dev/strait-mcp` | `apps/docs/mcp/` | `docs.strait.dev/mcp/*` |

## Triggering a sync from an SDK repo

Add this to the SDK repo's release or push workflow:

```yaml
- name: Trigger docs sync
  uses: peter-evans/repository-dispatch@v3
  with:
    token: ${{ secrets.DOCS_SYNC_TOKEN }}
    repository: strait-dev/strait
    event-type: sync-docs
```

Requires a `DOCS_SYNC_TOKEN` secret with `repo` scope on the SDK repo.

## Writing SDK docs

Each MDX file in `docs/` must have Mintlify frontmatter:

```yaml
---
title: "Page Title"
description: "One-line description."
---
```

Use Mintlify components: `<CodeGroup>`, `<Card>`, `<CardGroup>`, `<Tabs>`, `<Tab>`, `<Steps>`, `<Step>`.
