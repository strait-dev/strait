package cdc

import (
	"os"
	"strings"
	"testing"
	"time"

	straitcache "strait/internal/cache"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRequiredConsumerTablesReturnsCopy(t *testing.T) {
	t.Parallel()

	tables := RequiredConsumerTables()
	require.NotEmpty(t,
		tables)

	firstTable := tables[0]
	tables[0] = "public.mutated"
	require.Equal(t, firstTable,
		RequiredConsumerTables()[0])
}

func TestSequinConfigCoversRequiredConsumerTables(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../../packages/configs/sequin.yml")
	require.NoError(t,
		err)

	config := string(raw)
	for _, table := range RequiredConsumerTables() {
		table := tableName(t, table)
		require.Contains(t, config, `table_name: "`+table+`"`)
	}
}

func TestPostgresCDCInitSetsReplicaIdentityForRequiredConsumerTables(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../../packages/configs/postgres-init.sql")
	require.NoError(t,
		err)

	config := string(raw)
	for _, table := range RequiredConsumerTables() {
		table := tableName(t, table)
		require.Contains(t, config, "ALTER TABLE public."+table+" REPLICA IDENTITY FULL")
	}
}

func TestPostgresInitCreatesStraitAppRole(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../../packages/configs/postgres-init.sql")
	require.NoError(t, err)
	config := string(raw)
	for _, required := range []string{"CREATE ROLE strait_app", "NOLOGIN", "NOBYPASSRLS"} {
		require.Contains(t, config, required, "strait_app role contract")
	}
}

func TestRequiredConsumerTablesCoverRuntimeFanoutHandlers(t *testing.T) {
	t.Parallel()

	handlers := NewRuntimeFanoutHandlers(nil, nil)
	handlers = append(handlers,
		NewNotificationTriggerHandler(nil, nil),
		NewSLOHandler(nil, nil),
		NewAnalyticsHandler(nil, nil),
	)
	assertRequiredConsumerTablesIncludeHandlers(t, handlers)
}

func TestRuntimeFanoutHandlersExcludeWorkflowStepRuns(t *testing.T) {
	t.Parallel()

	for _, handler := range NewRuntimeFanoutHandlers(nil, nil) {
		require.NotEqual(t,
			"workflow_step_runs",

			handler.Table())
	}
}

func TestRequiredConsumerTablesCoverCacheReadModelHandlers(t *testing.T) {
	t.Parallel()

	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = rdb.Close() })

	handlers := NewCacheReadModelHandlers(rdb, time.Minute, nil)
	assertRequiredConsumerTablesIncludeHandlers(t, []Handler{
		handlers.JobRuns,
		handlers.WorkflowRuns,
		handlers.WorkflowStepRuns,
	})
}

func TestRequiredConsumerTablesCoverStrongCachePolicyTables(t *testing.T) {
	t.Parallel()

	required := requiredConsumerTableSet(t)
	for _, policy := range straitcache.StrongNamespacePolicies {
		for _, table := range policy.CDCTables {
			if _, ok := required[table]; !ok {
				require.Failf(t, "test failure",

					"strong cache policy %s requires CDC table %s but Sequin consumer does not subscribe to it", policy.Namespace, table)
			}
		}
	}
}

func assertRequiredConsumerTablesIncludeHandlers(t *testing.T, handlers []Handler) {
	t.Helper()

	required := requiredConsumerTableSet(t)
	for _, handler := range handlers {
		require.NotNil(t, handler)

		table := handler.Table()
		if _, ok := required[table]; !ok {
			require.Failf(t, "test failure",

				"runtime handler for %s is registered without required Sequin subscription", table)
		}
	}
}

func requiredConsumerTableSet(t *testing.T) map[string]struct{} {
	t.Helper()

	required := make(map[string]struct{}, len(RequiredConsumerTables()))
	for _, table := range RequiredConsumerTables() {
		required[tableName(t, table)] = struct{}{}
	}
	return required
}

func tableName(t *testing.T, qualifiedTable string) string {
	t.Helper()

	table, ok := strings.CutPrefix(qualifiedTable, "public.")
	require.True(t, ok)
	require.NotEmpty(t, table)

	return table
}
