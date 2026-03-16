# Changesets

This project uses [Changesets](https://github.com/changesets/changesets) to manage versioning and changelogs for all 5 SDKs.

## Adding a changeset

When you make changes to any SDK, run:

```sh
bunx changeset
```

This will prompt you to:
1. Select the affected packages
2. Choose a bump type (patch / minor / major)
3. Write a summary (becomes the changelog entry)

Commit the generated changeset file (in `.changeset/`) along with your code changes.

## How releases work

1. PRs with changesets are merged to `master`
2. The Changesets GitHub Action opens a "Version Packages" PR that bumps versions and updates changelogs
3. Merging the "Version Packages" PR triggers publishing of all 5 SDKs to their registries

All 5 SDKs (TypeScript, Python, Go, Ruby, Rust) are versioned together via the `fixed` config — a single changeset bumps all SDKs to the same version.
