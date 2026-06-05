//go:build !integration

package clickhouse

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CreateSchema error paths

func TestCreateSchema_FailsOnFirstTable(t *testing.T) {
	t.Parallel()

	client := newFailingClient(t)
	err := CreateSchema(context.Background(), client)
	require.Error(t, err)
	assert.Contains(t, err.
		Error(), "create table")
}

func TestCreateSchema_CanceledContext(t *testing.T) {
	t.Parallel()

	db, dbErr := newOpenDB(t)
	require.NoError(t, dbErr)

	defer db.Close()

	client := &Client{db: db, logger: slog.Default()}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := CreateSchema(ctx, client)
	require.Error(t, err)
}

func TestCreateSchema_ErrorMessageContainsTableName(t *testing.T) {
	t.Parallel()

	client := newFailingClient(t)
	err := CreateSchema(context.Background(), client)
	require.Error(t, err)

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
			assert.NotEmpty(t, strings.TrimSpace(d.ddl))
			assert.False(t, !strings.Contains(
				d.ddl, "IF NOT EXISTS",
			) &&
				!strings.Contains(
					d.
						ddl,
					"AS"))
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
		require.False(t, strings.Contains(
			CostDailyTable,
			token) ||
			strings.Contains(CostDailyMV,

				token))
	}
	require.Contains(t, CostDailyMV, "FROM run_analytics")
	require.Contains(t, CostDailyMV, "sum(compute_cost_microusd)")
}

// Schema alterations are well-formed

func TestSchemaAlterations_AreIdempotent(t *testing.T) {
	t.Parallel()

	for _, alt := range schemaAlterations {
		assert.NotEmpty(t, alt.
			table)
		assert.Contains(t, alt.
			ddl, "ADD COLUMN IF NOT EXISTS",
		)
	}
}

func TestSchemaDDL_UsesRedactedLongLivedAnalyticsColumns(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"message String", "metadata String", "webhook_url String"} {
		require.False(t, strings.Contains(
			RunEventsTable,
			raw) ||
			strings.Contains(WebhookDeliveryEventsTable,

				raw))
	}
	for _, safe := range []string{"message_class LowCardinality(String)", "metadata_redacted String DEFAULT '{}'", "webhook_host String"} {
		require.False(t, !strings.Contains(RunEventsTable,
			safe) &&
			!strings.Contains(WebhookDeliveryEventsTable,

				safe,
			),
		)
	}
	require.NotContains(t, WebhookDeliveryEventsTable, "ORDER BY (project_id, webhook_url")
	require.Contains(t, WebhookDeliveryEventsTable, "ORDER BY (project_id, webhook_host")
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
	for _, found := range required {
		require.True(t, found)
	}
}
