package cdc

import (
	"os"
	"strings"
	"testing"
	"time"

	straitcache "strait/internal/cache"

	"github.com/redis/go-redis/v9"
)

func TestRequiredConsumerTablesReturnsCopy(t *testing.T) {
	t.Parallel()

	tables := RequiredConsumerTables()
	if len(tables) == 0 {
		t.Fatal("required consumer tables must not be empty")
	}

	firstTable := tables[0]
	tables[0] = "public.mutated"
	if RequiredConsumerTables()[0] != firstTable {
		t.Fatal("required consumer tables must return a defensive copy")
	}
}

func TestSequinConfigCoversRequiredConsumerTables(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../../packages/configs/sequin.yml")
	if err != nil {
		t.Fatalf("read sequin config: %v", err)
	}
	config := string(raw)
	for _, table := range RequiredConsumerTables() {
		table := tableName(t, table)
		if !strings.Contains(config, `table_name: "`+table+`"`) {
			t.Fatalf("sequin config missing table %s", table)
		}
	}
}

func TestPostgresCDCInitSetsReplicaIdentityForRequiredConsumerTables(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../../packages/configs/postgres-init.sql")
	if err != nil {
		t.Fatalf("read postgres init config: %v", err)
	}
	config := string(raw)
	for _, table := range RequiredConsumerTables() {
		table := tableName(t, table)
		if !strings.Contains(config, "ALTER TABLE public."+table+" REPLICA IDENTITY FULL") {
			t.Fatalf("postgres init missing replica identity for %s", table)
		}
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
		if handler.Table() == "workflow_step_runs" {
			t.Fatal("workflow_step_runs fanout channel is launch-inactive; keep it covered by cache read-model handlers only")
		}
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
				t.Fatalf("strong cache policy %s requires CDC table %s but Sequin consumer does not subscribe to it", policy.Namespace, table)
			}
		}
	}
}

func assertRequiredConsumerTablesIncludeHandlers(t *testing.T, handlers []Handler) {
	t.Helper()

	required := requiredConsumerTableSet(t)
	for _, handler := range handlers {
		if handler == nil {
			t.Fatal("runtime handler must not be nil")
		}
		table := handler.Table()
		if _, ok := required[table]; !ok {
			t.Fatalf("runtime handler for %s is registered without required Sequin subscription", table)
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
	if !ok || table == "" {
		t.Fatalf("required consumer table %q must be public schema-qualified", qualifiedTable)
	}
	return table
}
