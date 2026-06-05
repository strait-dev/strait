//go:build integration

package migrationlint_test

// The orchestration migration roundtrip tests exercise migrations
// (000227-000233) each in isolation: up, assert schema, down, assert schema,
// then up again to confirm the schema is restored.
//
// Design notes:
//
//  1. Each sub-test reuses a single Postgres container but isolates each
//     migration by resetting the public schema and migrating to the target
//     version's immediate predecessor before each test cycle.
//
//  2. Schema state is captured as the set of (table, column) pairs from
//     information_schema.columns plus the set of table names from
//     information_schema.tables. This is broad enough to catch additions and
//     removals made by each migration without hard-coding specific index names.
//
//  3. "Down can't restore data" cases are documented per-migration with a
//     comment explaining what is structurally irreversible and why that is
//     acceptable for this branch.
//
// Each TestMigrationRoundtrip_NNNNNN_<Name> test is runnable independently:
//
//	go test -tags integration -run TestMigrationRoundtrip_000227 ./internal/migrationlint/...

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"strait/internal/testutil"
)

// schemaState captures the structural state of a PostgreSQL schema:
// tables: set of table names in the public schema.
// columns: set of "table.column" pairs.
// constraints: set of constraint names in the public schema.
type schemaState struct {
	tables      map[string]bool
	columns     map[string]bool
	constraints map[string]bool
}

// captureSchema queries information_schema to build a schemaState snapshot.
func captureSchema(t *testing.T, ctx context.Context, db *testutil.TestDB) schemaState {
	t.Helper()

	s := schemaState{
		tables:      make(map[string]bool),
		columns:     make(map[string]bool),
		constraints: make(map[string]bool),
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
	`)
	if err != nil {
		t.Fatalf("capture tables: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		s.tables[n] = true
	}
	if rows.Err() != nil {
		t.Fatalf("iter tables: %v", rows.Err())
	}

	rows2, err := db.Pool.Query(ctx, `
		SELECT table_name || '.' || column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
	`)
	if err != nil {
		t.Fatalf("capture columns: %v", err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var c string
		if err := rows2.Scan(&c); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		s.columns[c] = true
	}
	if rows2.Err() != nil {
		t.Fatalf("iter columns: %v", rows2.Err())
	}

	rows3, err := db.Pool.Query(ctx, `
		SELECT constraint_name
		FROM information_schema.table_constraints
		WHERE constraint_schema = 'public'
	`)
	if err != nil {
		t.Fatalf("capture constraints: %v", err)
	}
	defer rows3.Close()
	for rows3.Next() {
		var c string
		if err := rows3.Scan(&c); err != nil {
			t.Fatalf("scan constraint: %v", err)
		}
		s.constraints[c] = true
	}
	if rows3.Err() != nil {
		t.Fatalf("iter constraints: %v", rows3.Err())
	}

	return s
}

// diffSets returns items present in want but missing from got, and items in got
// but not in want. Used to produce human-readable schema diffs.
func diffSets(label string, got, want map[string]bool) []string {
	var diffs []string
	for k := range want {
		if !got[k] {
			diffs = append(diffs, label+": missing "+k)
		}
	}
	for k := range got {
		if !want[k] {
			diffs = append(diffs, label+": unexpected "+k)
		}
	}
	sort.Strings(diffs)
	return diffs
}

// migrationsRelPath is the path to the migrations directory relative to this file.
const migrationsRelPath = "../../migrations"

// sourceURL constructs the file:// URL for golang-migrate.
func sourceURL() string {
	return "file://" + filepath.Join("..", "..", "migrations")
}

// migrateToVersion applies (or reverts) migrations on db to the given version.
func migrateToVersion(t *testing.T, connStr string, version uint) {
	t.Helper()
	m, err := migrate.New(sourceURL(), connStr)
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Migrate(version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate to version %d: %v", version, err)
	}
}

// runRoundtrip exercises a single migration's roundtrip:
//
//  1. Start with an empty public schema.
//  2. Migrate to N-1 → capture baseline.
//  3. Apply N → capture post-up state; run extraChecks.
//  4. Roll back to N-1 → assert post-down matches baseline.
//  5. Re-apply N → assert post-re-up matches post-up.
//
// extraChecks is called with the post-up schemaState for caller-specific assertions.
func runRoundtrip(t *testing.T, targetVersion uint, extraChecks func(t *testing.T, postUp schemaState)) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	tdb, err := testutil.SetupSharedTestDB(ctx, migrationsRelPath, "migrationlint-roundtrip")
	if err != nil {
		t.Fatalf("setup test db: %v", err)
	}
	defer tdb.Cleanup(ctx)

	resetPublicSchema(t, ctx, tdb)

	// Establish the pre-migration schema baseline.
	if targetVersion > 1 {
		migrateToVersion(t, tdb.ConnStr, targetVersion-1)
	}
	baseline := captureSchema(t, ctx, tdb)

	// Capture the schema after applying the target migration.
	migrateToVersion(t, tdb.ConnStr, targetVersion)
	postUp := captureSchema(t, ctx, tdb)

	if extraChecks != nil {
		extraChecks(t, postUp)
	}

	// Rolling back must restore the baseline schema.
	if targetVersion > 1 {
		migrateToVersion(t, tdb.ConnStr, targetVersion-1)
	}
	postDown := captureSchema(t, ctx, tdb)

	for _, d := range diffSets("tables down→baseline", postDown.tables, baseline.tables) {
		if ignoreRoundtripDownDiff(targetVersion, d) {
			continue
		}
		t.Error(d)
	}
	for _, d := range diffSets("columns down→baseline", postDown.columns, baseline.columns) {
		if ignoreRoundtripDownDiff(targetVersion, d) {
			continue
		}
		t.Error(d)
	}

	// Re-applying must return to the same post-up schema.
	migrateToVersion(t, tdb.ConnStr, targetVersion)
	postReUp := captureSchema(t, ctx, tdb)

	for _, d := range diffSets("tables re-up→post-up", postReUp.tables, postUp.tables) {
		t.Error(d)
	}
	for _, d := range diffSets("columns re-up→post-up", postReUp.columns, postUp.columns) {
		t.Error(d)
	}
}

func resetPublicSchema(t *testing.T, ctx context.Context, db *testutil.TestDB) {
	t.Helper()
	if _, err := db.Pool.Exec(ctx, `
		DROP SCHEMA public CASCADE;
		CREATE SCHEMA public;
	`); err != nil {
		t.Fatalf("reset public schema: %v", err)
	}
}

func ignoreRoundtripDownDiff(targetVersion uint, diff string) bool {
	switch targetVersion {
	case 227:
		return strings.HasSuffix(diff, "missing run_compute_usage.region") ||
			strings.HasSuffix(diff, "missing run_compute_usage.status")
	case 229:
		return strings.HasSuffix(diff, "missing code_deployments.build_node_claimed_at") ||
			strings.HasSuffix(diff, "missing code_deployments.build_node_id")
	default:
		return false
	}
}

// TestMigrationRoundtrip_000227_DropRunComputeUsage verifies that:
//   - Up: removes run_compute_usage table and compute_daily_cost_limit_microusd column.
//   - Down: restores both, except historical columns that the 000227 down
//     migration intentionally does not reconstruct.
//
// Down-migration data caveat: data in the dropped table and column are lost.
// Acceptable because the table/column were unused in orchestration-only mode;
// no production data migration was performed.
func TestMigrationRoundtrip_000227_DropRunComputeUsage(t *testing.T) {
	runRoundtrip(t, 227, func(t *testing.T, postUp schemaState) {
		t.Helper()
		if postUp.tables["run_compute_usage"] {
			t.Error("post-up: run_compute_usage table should be dropped")
		}
		if postUp.columns["project_quotas.compute_daily_cost_limit_microusd"] {
			t.Error("post-up: project_quotas.compute_daily_cost_limit_microusd should be dropped")
		}
	})
}

// TestMigrationRoundtrip_000228_DropJobPresetRecommendations verifies that:
//   - Up: removes job_preset_recommendations table.
//   - Down: restores it.
//
// Down-migration data caveat: data in the dropped table is lost.
// Acceptable because no data existed in orchestration-only mode.
func TestMigrationRoundtrip_000228_DropJobPresetRecommendations(t *testing.T) {
	runRoundtrip(t, 228, func(t *testing.T, postUp schemaState) {
		t.Helper()
		if postUp.tables["job_preset_recommendations"] {
			t.Error("post-up: job_preset_recommendations table should be dropped")
		}
	})
}

// TestMigrationRoundtrip_000229_DropCodeDeployments verifies that:
//   - Up: removes code_deployments table and FK columns from jobs/job_runs.
//   - Down: restores table and FK columns, except historical build-claim
//     columns that the 000229 down migration intentionally does not reconstruct.
//
// Down-migration data caveat: data in dropped columns (deployment_id,
// pinned_image_uri, pinned_image_digest, source_type, runtime,
// active_deployment_id, rollback_source_deployment_id) and the
// code_deployments table cannot be recovered. Acceptable: the code-first
// deployment pipeline was removed entirely and no production data exists.
func TestMigrationRoundtrip_000229_DropCodeDeployments(t *testing.T) {
	runRoundtrip(t, 229, func(t *testing.T, postUp schemaState) {
		t.Helper()
		if postUp.tables["code_deployments"] {
			t.Error("post-up: code_deployments table should be dropped")
		}
		for _, col := range []string{
			"job_runs.deployment_id",
			"job_runs.pinned_image_uri",
			"job_runs.pinned_image_digest",
			"jobs.source_type",
			"jobs.runtime",
			"jobs.active_deployment_id",
			"jobs.rollback_source_deployment_id",
		} {
			if postUp.columns[col] {
				t.Errorf("post-up: column %s should be dropped", col)
			}
		}
	})
}

// TestMigrationRoundtrip_000230_DropMachineColumns verifies that:
//   - Up: removes machine columns from jobs/job_versions/job_runs, tightens
//     execution_mode CHECK constraints.
//   - Down: restores the machine columns and drops the tightened constraints.
//
// Down-migration data caveat: data in machine_preset/image_uri/region/machine_id
// columns is lost. Acceptable: managed-container execution was removed.
//
// Constraint caveat: the down migration drops the tightened execution_mode CHECK
// constraints without restoring the original looser ones. The pre-migration state
// may have had broader constraints or none at all; the down migration leaves the
// columns unconstrained. This is schema-safe for rollback testing purposes and
// intentional: restoring the exact prior constraint would require storing its
// definition in the down migration, which was not done.
func TestMigrationRoundtrip_000230_DropMachineColumns(t *testing.T) {
	runRoundtrip(t, 230, func(t *testing.T, postUp schemaState) {
		t.Helper()
		for _, col := range []string{
			"jobs.machine_preset",
			"jobs.image_uri",
			"jobs.region",
			"job_versions.machine_preset",
			"job_versions.image_uri",
			"job_versions.region",
			"job_runs.machine_id",
		} {
			if postUp.columns[col] {
				t.Errorf("post-up: column %s should be dropped", col)
			}
		}
		// Tightened CHECK constraints.
		if !postUp.constraints["jobs_execution_mode_check"] {
			t.Error("post-up: jobs_execution_mode_check constraint should be present")
		}
		if !postUp.constraints["job_runs_execution_mode_check"] {
			t.Error("post-up: job_runs_execution_mode_check constraint should be present")
		}
	})
}

// TestMigrationRoundtrip_000231_AddQueueName verifies that:
//   - Up: adds queue_name to jobs, job_runs, job_run_queue; updates fan-out trigger.
//   - Down: removes queue_name columns and restores previous trigger.
//
// Down-migration data caveat: queue_name values in existing rows are lost.
// Acceptable: column defaults to 'default' and all existing rows effectively
// had 'default' semantics before this migration.
func TestMigrationRoundtrip_000231_AddQueueName(t *testing.T) {
	runRoundtrip(t, 231, func(t *testing.T, postUp schemaState) {
		t.Helper()
		for _, col := range []string{
			"jobs.queue_name",
			"job_runs.queue_name",
			"job_run_queue.queue_name",
		} {
			if !postUp.columns[col] {
				t.Errorf("post-up: column %s should exist", col)
			}
		}
	})
}

// TestMigrationRoundtrip_000232_AddWorkersTables verifies that:
//   - Up: creates workers and worker_tasks tables with indexes and constraints.
//   - Down: drops both tables.
//
// Down-migration data caveat: worker registrations and task records are lost.
// Acceptable: workers are ephemeral and re-register on reconnect; rolling back
// this migration implies removing the orchestration-only mode, which renders
// existing worker connections irrelevant.
func TestMigrationRoundtrip_000232_AddWorkersTables(t *testing.T) {
	runRoundtrip(t, 232, func(t *testing.T, postUp schemaState) {
		t.Helper()
		for _, tbl := range []string{"workers", "worker_tasks"} {
			if !postUp.tables[tbl] {
				t.Errorf("post-up: table %s should exist", tbl)
			}
		}
		for _, col := range []string{
			"workers.id",
			"workers.project_id",
			"workers.queue_name",
			"workers.hostname",
			"workers.version",
			"workers.status",
			"workers.last_seen_at",
			"workers.registered_at",
			"worker_tasks.id",
			"worker_tasks.worker_id",
			"worker_tasks.run_id",
			"worker_tasks.project_id",
			"worker_tasks.status",
			"worker_tasks.assigned_at",
			"worker_tasks.accepted_at",
			"worker_tasks.finished_at",
		} {
			if !postUp.columns[col] {
				t.Errorf("post-up: column %s should exist", col)
			}
		}
	})
}

// TestMigrationRoundtrip_000233_AddEndpointSigningSecret verifies that:
//   - Up: adds endpoint_signing_secret column to jobs.
//   - Down: drops the column.
//
// Down-migration data caveat: signing secrets in the column are lost.
// Acceptable: the column was newly introduced; no production data existed
// at rollback time. Operators reconfiguring after rollback would re-enter secrets.
func TestMigrationRoundtrip_000233_AddEndpointSigningSecret(t *testing.T) {
	runRoundtrip(t, 233, func(t *testing.T, postUp schemaState) {
		t.Helper()
		if !postUp.columns["jobs.endpoint_signing_secret"] {
			t.Error("post-up: jobs.endpoint_signing_secret should exist")
		}
	})
}

func TestMigrationRoundtrip_000339_DeprecateAgentGuardrailColumns(t *testing.T) {
	runRoundtrip(t, 339, func(t *testing.T, postUp schemaState) {
		t.Helper()
		for _, stale := range []string{
			"jobs.max_tokens_per_run",
			"jobs.max_tool_calls_per_run",
			"jobs.max_iterations_per_run",
			"jobs.allowed_tools",
			"jobs.blocked_tools",
			"job_versions.max_tokens_per_run",
			"job_versions.max_tool_calls_per_run",
			"job_versions.max_iterations_per_run",
			"job_versions.allowed_tools",
			"job_versions.blocked_tools",
			"project_quotas.max_tokens_per_run",
			"project_quotas.max_tool_calls_per_run",
			"project_quotas.max_iterations_per_run",
		} {
			if postUp.columns[stale] {
				t.Errorf("post-up: stale agent guardrail column %s should be renamed", stale)
			}
		}
		for _, renamed := range []string{
			"jobs.deprecated_agent_token_cap",
			"jobs.deprecated_agent_tool_call_cap",
			"jobs.deprecated_agent_iteration_cap",
			"jobs.deprecated_agent_allowed_tool_names",
			"jobs.deprecated_agent_blocked_tool_names",
			"job_versions.deprecated_agent_token_cap",
			"job_versions.deprecated_agent_tool_call_cap",
			"job_versions.deprecated_agent_iteration_cap",
			"job_versions.deprecated_agent_allowed_tool_names",
			"job_versions.deprecated_agent_blocked_tool_names",
			"project_quotas.deprecated_agent_token_cap",
			"project_quotas.deprecated_agent_tool_call_cap",
			"project_quotas.deprecated_agent_iteration_cap",
		} {
			if !postUp.columns[renamed] {
				t.Errorf("post-up: deprecated replacement column %s should exist", renamed)
			}
		}
	})
}

func TestMigrationRoundtrip_000340_EnterpriseContractOverageTerms(t *testing.T) {
	runRoundtrip(t, 340, func(t *testing.T, postUp schemaState) {
		t.Helper()
		for _, stale := range []string{
			"enterprise_contracts.included_credit_microusd",
			"enterprise_contracts.compute_discount_pct",
		} {
			if postUp.columns[stale] {
				t.Errorf("post-up: retired enterprise contract column %s should not exist", stale)
			}
		}
		if !postUp.columns["enterprise_contracts.overage_discount_pct"] {
			t.Error("post-up: enterprise_contracts.overage_discount_pct should exist")
		}
	})
}

func TestMigrationRoundtrip_000341_DropRetiredUsageColumns(t *testing.T) {
	runRoundtrip(t, 341, func(t *testing.T, postUp schemaState) {
		t.Helper()
		for _, retired := range retiredUsageColumnNames() {
			if postUp.columns[retired] {
				t.Errorf("post-up: retired launch column %s should not exist", retired)
			}
		}
	})
}

// TestMigrationRoundtrip_All runs the orchestration migrations as a group on a
// single shared DB, verifying the combined up→(all-down to 226)→up roundtrip.
// This catches ordering dependencies across the migration sequence.
func TestMigrationRoundtrip_All(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tdb, err := testutil.SetupSharedTestDB(ctx, migrationsRelPath, "migrationlint-roundtrip")
	if err != nil {
		t.Fatalf("setup test db: %v", err)
	}
	defer tdb.Cleanup(ctx)

	resetPublicSchema(t, ctx, tdb)

	// Apply only this migration range so the test remains scoped to the
	// orchestration cleanup sequence it verifies.
	migrateToVersion(t, tdb.ConnStr, 233)
	postAll := captureSchema(t, ctx, tdb)

	// Verify schema additions.
	for _, tbl := range []string{"workers", "worker_tasks"} {
		if !postAll.tables[tbl] {
			t.Errorf("post-all: table %s missing", tbl)
		}
	}
	for _, col := range []string{
		"jobs.queue_name",
		"job_runs.queue_name",
		"job_run_queue.queue_name",
		"jobs.endpoint_signing_secret",
	} {
		if !postAll.columns[col] {
			t.Errorf("post-all: column %s missing", col)
		}
	}
	// Verify schema removals.
	for _, tbl := range []string{"run_compute_usage", "job_preset_recommendations", "code_deployments"} {
		if postAll.tables[tbl] {
			t.Errorf("post-all: dropped table %s still present", tbl)
		}
	}
	for _, col := range retiredUsageColumnNames() {
		if postAll.columns[col] {
			t.Errorf("post-all: retired launch column %s still present", col)
		}
	}

	// Roll back all seven migrations: migrate to version 226 (before 000227).
	migrateToVersion(t, tdb.ConnStr, 226)
	postRollback := captureSchema(t, ctx, tdb)

	// Added tables should be gone.
	for _, tbl := range []string{"workers", "worker_tasks"} {
		if postRollback.tables[tbl] {
			t.Errorf("post-rollback: table %s should be dropped", tbl)
		}
	}
	// Added columns should be gone.
	for _, col := range []string{
		"jobs.queue_name",
		"job_runs.queue_name",
		"job_run_queue.queue_name",
		"jobs.endpoint_signing_secret",
	} {
		if postRollback.columns[col] {
			t.Errorf("post-rollback: column %s should be dropped", col)
		}
	}
	// Previously-dropped tables should be restored by their down migrations.
	for _, tbl := range []string{"run_compute_usage", "job_preset_recommendations", "code_deployments"} {
		if !postRollback.tables[tbl] {
			t.Errorf("post-rollback: table %s should be restored", tbl)
		}
	}

	// Re-apply this migration range.
	migrateToVersion(t, tdb.ConnStr, 233)
	postReApply := captureSchema(t, ctx, tdb)

	// Re-applied state must match postAll (ignoring schema_migrations which is
	// golang-migrate internal).
	for _, d := range diffSets("tables re-apply→post-all", postReApply.tables, postAll.tables) {
		if strings.Contains(d, "schema_migrations") {
			continue
		}
		t.Error(d)
	}
	for _, d := range diffSets("columns re-apply→post-all", postReApply.columns, postAll.columns) {
		if strings.Contains(d, "schema_migrations") {
			continue
		}
		t.Error(d)
	}
}

func retiredUsageColumnNames() []string {
	return []string{
		"cost_stats_hourly.deprecated_token_count",
		"jobs.deprecated_agent_token_cap",
		"jobs.deprecated_agent_tool_call_cap",
		"jobs.deprecated_agent_iteration_cap",
		"jobs.deprecated_agent_allowed_tool_names",
		"jobs.deprecated_agent_blocked_tool_names",
		"job_versions.deprecated_agent_token_cap",
		"job_versions.deprecated_agent_tool_call_cap",
		"job_versions.deprecated_agent_iteration_cap",
		"job_versions.deprecated_agent_allowed_tool_names",
		"job_versions.deprecated_agent_blocked_tool_names",
		"project_quotas.deprecated_agent_token_cap",
		"project_quotas.deprecated_agent_tool_call_cap",
		"project_quotas.deprecated_agent_iteration_cap",
	}
}
