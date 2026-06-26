## Summary

<!-- What does this PR do? Be specific. -->

## Why

<!-- Context and motivation. Link the GitHub issue if applicable. -->

## What changed

<!-- Group changes by area. Example:
- **api/**: Added GET /v1/jobs/{id}/health endpoint
- **worker/**: Fixed retry delay calculation for custom strategies
- **store/**: New index on job_runs for heartbeat queries
-->

## How to test

<!-- Exact commands or steps to verify this works. -->

## Checklist

- [ ] `go build ./...` passes (both editions if applicable)
- [ ] `go test ./...` passes
- [ ] `golangci-lint run` passes
- [ ] Tests added or updated for new behavior
- [ ] Docs updated (if behavior, API, or config changed)
- [ ] OpenAPI spec updated (if endpoints changed)
- [ ] Migration includes both up and down files (if schema changed)
- [ ] No secrets, credentials, customer data, or private runbooks included
- [ ] No breaking changes (or noted below)

## Breaking changes

<!-- List any breaking changes. If none, delete this section.

If there are breaking changes, also mark the commit with `!` (e.g.
`feat(api)!: drop legacy /v0/jobs endpoint`) AND include a footer of
the form:

  BREAKING CHANGE: <one-line description for the changelog>

release-please surfaces these at the top of the release entry. -->

## Release notes

<!-- User-facing prose for the changelog. If the commit subject is
already a clear, one-line user-facing summary, leave this blank.

Otherwise write 1-3 sentences describing what this PR changes from a
user's perspective: what they can now do (or do differently), what
they should watch for after upgrading. This text lands in the squash
commit body and release-please includes it under the changelog
entry.

Skip for: refactors, internal infrastructure, test-only changes, dependency
bumps. Required for: feat / fix / perf / breaking. -->

## Risks and follow-ups

<!-- Anything reviewers should watch for. Optional. -->
