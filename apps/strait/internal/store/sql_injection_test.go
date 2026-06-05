package store

import (
	"errors"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper: assert that calling UpdateLogDrain with the given patch returns a
// FieldError for the offending column name.

func assertLogDrainFieldError(t *testing.T, patch map[string]any, wantField string) {
	t.Helper()
	q := &Queries{} // nil db -- we expect rejection before any SQL execution
	err := q.UpdateLogDrain(t.Context(), "drain-1", "proj-1", patch)
	require.Error(
		t, err)

	var fe *domain.FieldError
	require.True(t,
		errors.As(err, &fe))
	require.Equal(
		t, wantField,
		fe.Field,
	)

}

func assertEventSourceFieldError(t *testing.T, patch map[string]any, wantField string) {
	t.Helper()
	q := &Queries{} // nil db -- we expect rejection before any SQL execution
	err := q.UpdateEventSource(t.Context(), "src-1", "proj-1", patch)
	require.Error(
		t, err)

	var fe *domain.FieldError
	require.True(t,
		errors.As(err, &fe))
	require.Equal(
		t, wantField,
		fe.Field,
	)

}

// UpdateLogDrain: SQL injection vectors

func TestUpdateLogDrain_RejectsInjectedColumnName(t *testing.T) {
	t.Parallel()
	assertLogDrainFieldError(t, map[string]any{
		"name; DROP TABLE log_drains --": "x",
	}, "name; DROP TABLE log_drains --")
}

func TestUpdateLogDrain_RejectsSQLComment(t *testing.T) {
	t.Parallel()
	assertLogDrainFieldError(t, map[string]any{"name--": "x"}, "name--")
}

func TestUpdateLogDrain_RejectsSubquery(t *testing.T) {
	t.Parallel()
	assertLogDrainFieldError(t, map[string]any{"(SELECT 1)": "x"}, "(SELECT 1)")
}

func TestUpdateLogDrain_RejectsEmptyColumn(t *testing.T) {
	t.Parallel()
	assertLogDrainFieldError(t, map[string]any{"": "x"}, "")
}

func TestUpdateLogDrain_AcceptsAllValidColumns(t *testing.T) {
	t.Parallel()
	validCols := []string{
		"name", "drain_type", "endpoint_url", "auth_type",
		"auth_config", "level_filter", "enabled",
	}
	for _, col := range validCols {
		t.Run(col, func(t *testing.T) {
			t.Parallel()
			func() {
				defer func() { _ = recover() }()
				q := &Queries{}
				err := q.UpdateLogDrain(t.Context(), "drain-1", "proj-1", map[string]any{col: "val"})
				if err == nil {
					return
				}
				var fe *domain.FieldError
				require.False(
					t, errors.As(err, &fe),
				)

			}()
		})
	}
}

func TestUpdateLogDrain_RejectsUnknownColumn(t *testing.T) {
	t.Parallel()
	assertLogDrainFieldError(t, map[string]any{"admin_access": true}, "admin_access")
}

func TestUpdateLogDrain_MixedValidInvalid(t *testing.T) {
	t.Parallel()
	// Due to map iteration order, either the valid or invalid key may be hit first.
	// The function MUST reject if ANY key is invalid.
	q := &Queries{}
	err := q.UpdateLogDrain(t.Context(), "drain-1", "proj-1", map[string]any{
		"name":         "ok",
		"admin_access": true,
	})
	require.Error(
		t, err)

	var fe *domain.FieldError
	if !errors.As(err, &fe) {
		// Acceptable: could be nil db panic if valid key was processed first.
		// But if we get a FieldError it must be for the bad key.
		return
	}
	require.Equal(
		t, "admin_access",
		fe.
			Field,
	)

}

// UpdateEventSource: SQL injection vectors

func TestUpdateEventSource_RejectsInjectedColumnName(t *testing.T) {
	t.Parallel()
	assertEventSourceFieldError(t, map[string]any{
		"name; DROP TABLE event_sources --": "x",
	}, "name; DROP TABLE event_sources --")
}

func TestUpdateEventSource_RejectsSQLComment(t *testing.T) {
	t.Parallel()
	assertEventSourceFieldError(t, map[string]any{"name--": "x"}, "name--")
}

func TestUpdateEventSource_RejectsSubquery(t *testing.T) {
	t.Parallel()
	assertEventSourceFieldError(t, map[string]any{"(SELECT 1)": "x"}, "(SELECT 1)")
}

func TestUpdateEventSource_AcceptsAllValidColumns(t *testing.T) {
	t.Parallel()
	validCols := []string{
		"name", "description", "schema", "enabled",
		"signature_header", "signature_algorithm", "signature_secret_enc",
	}
	for _, col := range validCols {
		t.Run(col, func(t *testing.T) {
			t.Parallel()
			func() {
				defer func() { _ = recover() }()
				q := &Queries{}
				err := q.UpdateEventSource(t.Context(), "src-1", "proj-1", map[string]any{col: "val"})
				if err == nil {
					return
				}
				var fe *domain.FieldError
				require.False(
					t, errors.As(err, &fe),
				)

			}()
		})
	}
}

func TestUpdateEventSource_RejectsUnknownColumn(t *testing.T) {
	t.Parallel()
	assertEventSourceFieldError(t, map[string]any{"admin_access": true}, "admin_access")
}

// Verify existing allowlists still work (UpdateRunStatus, step runs, workflow runs)

func TestUpdateRunStatus_AllowlistRejectsUnknown(t *testing.T) {
	t.Parallel()
	q := &Queries{}
	err := q.UpdateRunStatus(t.Context(), "run-1", domain.StatusQueued, domain.StatusDequeued, map[string]any{
		"admin_column": "hack",
	})
	require.Error(
		t, err)

	var fe *domain.FieldError
	require.True(t,
		errors.As(err, &fe))
	require.Equal(
		t, "admin_column",
		fe.
			Field,
	)

}

func TestUpdateRunStatus_AllowlistAcceptsAllKnown(t *testing.T) {
	t.Parallel()
	knownCols := []string{
		"attempt", "payload", "result", "error", "error_class",
		"triggered_by", "scheduled_at", "started_at", "finished_at",
		"heartbeat_at", "next_retry_at", "expires_at", "execution_trace",
		"workflow_step_run_id", "debug_mode", "continuation_of",
		"lineage_depth", "priority", "metadata",
	}
	for _, col := range knownCols {
		t.Run(col, func(t *testing.T) {
			t.Parallel()
			// With nil db, the function will panic after passing the allowlist.
			// A panic means the column was accepted (good). A FieldError means rejected (bad).
			func() {
				defer func() { _ = recover() }()
				q := &Queries{}
				err := q.UpdateRunStatus(t.Context(), "run-1", domain.StatusQueued, domain.StatusDequeued, map[string]any{
					col: "val",
				})
				if err == nil {
					return
				}
				var fe *domain.FieldError
				require.False(
					t, errors.As(err, &fe),
				)

			}()
		})
	}
}

func TestUpdateStepRunStatus_AllowlistRejectsUnknown(t *testing.T) {
	t.Parallel()
	q := &Queries{}
	err := q.UpdateStepRunStatus(t.Context(), "step-1", domain.StepRunning, map[string]any{
		"admin_column": "hack",
	})
	require.Error(
		t, err)

	var fe *domain.FieldError
	require.True(t,
		errors.As(err, &fe))

}

func TestUpdateWorkflowRunStatus_AllowlistRejectsUnknown(t *testing.T) {
	t.Parallel()
	q := &Queries{}
	err := q.UpdateWorkflowRunStatus(t.Context(), "wf-1", domain.WfStatusPending, domain.WfStatusRunning, map[string]any{
		"admin_column": "hack",
	})
	require.Error(
		t, err)

	var fe *domain.FieldError
	require.True(t,
		errors.As(err, &fe))

}

// Fuzz: column name strings for UpdateLogDrain

func FuzzUpdateLogDrainColumnName(f *testing.F) {
	f.Add("name")
	f.Add("admin")
	f.Add("'; DROP TABLE--")
	f.Add("name; DELETE FROM log_drains")
	f.Add("(SELECT 1)")
	f.Add("$1")
	f.Add("")
	f.Add("\x00")
	f.Add("name\x00injected")

	f.Fuzz(func(t *testing.T, colName string) {
		allowed := map[string]bool{
			"name": true, "drain_type": true, "endpoint_url": true,
			"auth_type": true, "auth_config": true, "level_filter": true,
			"enabled": true, "updated_at": true,
		}

		var resultErr error
		panicked := true
		func() {
			defer func() { _ = recover() }()
			q := &Queries{}
			resultErr = q.UpdateLogDrain(t.Context(), "drain-1", "proj-1", map[string]any{colName: "val"})
			panicked = false
		}()

		if allowed[colName] {
			// Allowed columns: should NOT return a FieldError (panic from nil db is fine).
			var fe *domain.FieldError
			assert.False(t,
				!panicked &&
					errors.As(resultErr, &fe))

		} else {
			// Disallowed columns: must return a FieldError (never reach SQL).
			var fe *domain.FieldError
			if panicked {
				assert.Failf(t, "test failure",

					"disallowed column %q caused a panic instead of FieldError", colName)
			} else if !errors.As(resultErr, &fe) {
				assert.NotNil(
					t, resultErr,
				)

			}
		}
	})
}

// Fuzz: column name strings for UpdateEventSource

func FuzzUpdateEventSourceColumnName(f *testing.F) {
	f.Add("name")
	f.Add("admin")
	f.Add("'; DROP TABLE--")
	f.Add("(SELECT 1)")
	f.Add("$1")
	f.Add("")
	f.Add("\x00")

	f.Fuzz(func(t *testing.T, colName string) {
		allowed := map[string]bool{
			"name": true, "description": true, "schema": true,
			"enabled": true, "signature_header": true,
			"signature_algorithm": true, "signature_secret_enc": true,
			"updated_at": true,
		}

		var resultErr error
		panicked := true
		func() {
			defer func() { _ = recover() }()
			q := &Queries{}
			resultErr = q.UpdateEventSource(t.Context(), "src-1", "proj-1", map[string]any{colName: "val"})
			panicked = false
		}()

		if allowed[colName] {
			var fe *domain.FieldError
			assert.False(t,
				!panicked &&
					errors.As(resultErr, &fe))

		} else {
			var fe *domain.FieldError
			if panicked {
				assert.Failf(t, "test failure",

					"disallowed column %q caused a panic instead of FieldError", colName)
			} else if !errors.As(resultErr, &fe) {
				assert.NotNil(
					t, resultErr,
				)

			}
		}
	})
}

// DynamicUpdate adversarial column name patterns

func TestDynamicUpdate_ManyKeys(t *testing.T) {
	t.Parallel()
	patch := make(map[string]any, 100)
	for i := range 100 {
		patch["injected_col_"+strings.Repeat("x", i)] = "val"
	}
	q := &Queries{}
	err := q.UpdateLogDrain(t.Context(), "drain-1", "proj-1", patch)
	require.Error(
		t, err)

	var fe *domain.FieldError
	require.True(t,
		errors.As(err, &fe))

}

func TestDynamicUpdate_NullByteInColumn(t *testing.T) {
	t.Parallel()
	assertLogDrainFieldError(t, map[string]any{"name\x00injected": "x"}, "name\x00injected")
}

func TestDynamicUpdate_UnicodeColumn(t *testing.T) {
	t.Parallel()
	assertLogDrainFieldError(t, map[string]any{"\u0410\u0411\u0412": "x"}, "\u0410\u0411\u0412")
}

func TestDynamicUpdate_WhitespaceColumn(t *testing.T) {
	t.Parallel()
	assertLogDrainFieldError(t, map[string]any{"name\t\n": "x"}, "name\t\n")
}

func TestDynamicUpdate_EqualsSignColumn(t *testing.T) {
	t.Parallel()
	assertLogDrainFieldError(t, map[string]any{"name = 'hacked'": "x"}, "name = 'hacked'")
}

func TestDynamicUpdate_DollarSignColumn(t *testing.T) {
	t.Parallel()
	assertLogDrainFieldError(t, map[string]any{"$1": "x"}, "$1")
}

func TestDynamicUpdate_SemicolonColumn(t *testing.T) {
	t.Parallel()
	assertLogDrainFieldError(t, map[string]any{"name;": "x"}, "name;")
}
