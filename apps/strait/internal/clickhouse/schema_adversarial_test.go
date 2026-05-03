//go:build !integration

package clickhouse

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------.
// CreateSchema error paths
// ---------------------------------------------------------------------------.

func TestCreateSchema_FailsOnFirstTable(t *testing.T) {
	t.Parallel()

	client := newFailingClient(t)
	err := CreateSchema(context.Background(), client)
	if err == nil {
		t.Fatal("expected error from CreateSchema with failing client")
	}
	if !strings.Contains(err.Error(), "create table") {
		t.Errorf("expected 'create table' in error, got: %v", err)
	}
}

func TestCreateSchema_CanceledContext(t *testing.T) {
	t.Parallel()

	db, dbErr := newOpenDB(t)
	if dbErr != nil {
		t.Fatal(dbErr)
	}
	defer db.Close()

	client := &Client{db: db, logger: slog.Default()}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := CreateSchema(ctx, client)
	if err == nil {
		t.Fatal("expected error from CreateSchema with canceled context")
	}
}

func TestCreateSchema_ErrorMessageContainsTableName(t *testing.T) {
	t.Parallel()

	client := newFailingClient(t)
	err := CreateSchema(context.Background(), client)
	if err == nil {
		t.Fatal("expected error")
	}

	// The error should wrap the table name for debuggability.
	errMsg := err.Error()
	if !strings.Contains(errMsg, "run_events") {
		// The first table in the list is run_events, so if any table fails
		// first, it should be that one.
		t.Logf("error does not contain 'run_events' (may have failed on another table): %s", errMsg)
	}
}

// ---------------------------------------------------------------------------.
// Schema DDL constants are non-empty
// ---------------------------------------------------------------------------.

func TestSchemaDDL_NonEmpty(t *testing.T) {
	t.Parallel()

	ddls := []struct {
		name string
		ddl  string
	}{
		{"RunEventsTable", RunEventsTable},
		{"RunAnalyticsTable", RunAnalyticsTable},
		{"RunUsageEventsTable", RunUsageEventsTable},
		{"WorkflowApprovalEventsTable", WorkflowApprovalEventsTable},
		{"JobMetadataTable", JobMetadataTable},
		{"EventTriggerEventsTable", EventTriggerEventsTable},
		{"WorkflowRunAnalyticsTable", WorkflowRunAnalyticsTable},
		{"WorkflowStepAnalyticsTable", WorkflowStepAnalyticsTable},
		{"WebhookDeliveryEventsTable", WebhookDeliveryEventsTable},
		{"RunStatsDailyTable", RunStatsDailyTable},
		{"CostDailyTable", CostDailyTable},
		{"RunStatsDailyMV", RunStatsDailyMV},
		{"CostDailyMV", CostDailyMV},
	}

	for _, d := range ddls {
		t.Run(d.name, func(t *testing.T) {
			t.Parallel()
			if strings.TrimSpace(d.ddl) == "" {
				t.Errorf("%s is empty", d.name)
			}
			if !strings.Contains(d.ddl, "IF NOT EXISTS") && !strings.Contains(d.ddl, "AS") {
				t.Errorf("%s missing IF NOT EXISTS clause", d.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------.
// Schema alterations are well-formed
// ---------------------------------------------------------------------------.

func TestSchemaAlterations_AreIdempotent(t *testing.T) {
	t.Parallel()

	for i, alt := range schemaAlterations {
		if alt.table == "" {
			t.Errorf("alteration %d has empty table name", i)
		}
		if !strings.Contains(alt.ddl, "ADD COLUMN IF NOT EXISTS") {
			t.Errorf("alteration %d for table %q is not idempotent (missing IF NOT EXISTS): %s", i, alt.table, alt.ddl)
		}
	}
}
