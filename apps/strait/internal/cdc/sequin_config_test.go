package cdc

import (
	"os"
	"strings"
	"testing"
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

func tableName(t *testing.T, qualifiedTable string) string {
	t.Helper()

	table, ok := strings.CutPrefix(qualifiedTable, "public.")
	if !ok || table == "" {
		t.Fatalf("required consumer table %q must be public schema-qualified", qualifiedTable)
	}
	return table
}
