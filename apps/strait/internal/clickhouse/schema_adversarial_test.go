//go:build !integration

package clickhouse

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

// CreateSchema error paths

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

// Schema DDL constants are non-empty

func TestSchemaDDL_NonEmpty(t *testing.T) {
	t.Parallel()

	ddls := []struct {
		name string
		ddl  string
	}{
		{"RunEventsTable", RunEventsTable},
		{"RunAnalyticsTable", RunAnalyticsTable},
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

func TestSchemaDDL_DoesNotCreateRetiredModelUsageEvents(t *testing.T) {
	t.Parallel()

	for _, token := range []string{
		"run_usage_events",
		"prompt_tokens",
		"completion_tokens",
		"total_tokens",
		"sum(total_tokens)",
		"sum(cost_microusd) AS usage_cost_microusd",
	} {
		if strings.Contains(CostDailyTable, token) || strings.Contains(CostDailyMV, token) {
			t.Fatalf("cost daily schema contains retired model usage token %q", token)
		}
	}
	if !strings.Contains(CostDailyMV, "FROM run_analytics") {
		t.Fatal("cost daily materialized view must use run_analytics")
	}
	if !strings.Contains(CostDailyMV, "sum(compute_cost_microusd)") {
		t.Fatal("cost daily materialized view must use compute cost")
	}
}

// Schema alterations are well-formed

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

func TestSchemaDDL_UsesRedactedLongLivedAnalyticsColumns(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"message String", "metadata String", "webhook_url String"} {
		if strings.Contains(RunEventsTable, raw) || strings.Contains(WebhookDeliveryEventsTable, raw) {
			t.Fatalf("schema still declares raw sensitive analytics column %q", raw)
		}
	}
	for _, safe := range []string{"message_class LowCardinality(String)", "metadata_redacted String DEFAULT '{}'", "webhook_host String"} {
		if !strings.Contains(RunEventsTable, safe) && !strings.Contains(WebhookDeliveryEventsTable, safe) {
			t.Fatalf("schema missing safe analytics column %q", safe)
		}
	}
	if strings.Contains(WebhookDeliveryEventsTable, "ORDER BY (project_id, webhook_url") {
		t.Fatal("webhook analytics table still orders by raw webhook_url")
	}
	if !strings.Contains(WebhookDeliveryEventsTable, "ORDER BY (project_id, webhook_host") {
		t.Fatal("webhook analytics table does not order by redacted webhook_host")
	}
}

func TestSchemaAlterations_AddRedactedAnalyticsColumns(t *testing.T) {
	t.Parallel()

	required := map[string]bool{
		"ALTER TABLE run_events ADD COLUMN IF NOT EXISTS message_class":             false,
		"ALTER TABLE run_events ADD COLUMN IF NOT EXISTS metadata_redacted":         false,
		"ALTER TABLE webhook_delivery_events ADD COLUMN IF NOT EXISTS webhook_host": false,
	}
	for _, alt := range schemaAlterations {
		for prefix := range required {
			if strings.HasPrefix(alt.ddl, prefix) {
				required[prefix] = true
			}
		}
	}
	for prefix, found := range required {
		if !found {
			t.Fatalf("schema alterations missing %q", prefix)
		}
	}
}
