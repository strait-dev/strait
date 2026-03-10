# Migration Strategy

## Current Policy

- Existing numbered migrations are **immutable** once merged.
- Never rewrite or squash already-applied migrations in deployed environments.
- Backward/forward compatibility is maintained by additive migrations.

## Consolidation Plan for New Environments

To speed up fresh installs while preserving safety for existing deployments:

1. Keep historical migrations (000001..current) for upgrades.
2. Introduce a periodic **baseline snapshot** migration for fresh databases only.
3. Keep incremental migrations after the baseline for active development.

Example (next major baseline):

- `001000_baseline_schema.up.sql` — complete schema as of release cut
- `001001_*.sql` onward — normal incremental migrations

## Operational Rules

- Existing environments:
  - Continue applying incremental migrations in order.
  - Never skip historical migration state.
- New environments:
  - Bootstrap from baseline + subsequent increments.

## Tooling Notes

- Baseline generation should be automated from a migrated reference DB.
- CI should test both paths:
  1. from empty DB through full historical chain
  2. from baseline through latest increments

## Safety Checklist Before Releasing a Baseline

- [ ] Full schema diff against reference DB is empty
- [ ] Integration/E2E pass on baseline bootstrap path
- [ ] Integration/E2E pass on historical upgrade path
- [ ] Rollback scripts verified for latest increment set
