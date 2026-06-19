package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fillWorkflowSnapshotDest(dest []any, id string, createdAt time.Time) {
	*(dest[0].(*string)) = id
	*(dest[1].(*string)) = "wf-1"
	*(dest[2].(*string)) = "version-1"
	*(dest[3].(*int)) = 3
	*(dest[4].(*[]byte)) = []byte(`{"workflow":{"id":"wf-1"},"steps":[]}`)
	*(dest[5].(*time.Time)) = createdAt
}

func testWorkflowForSnapshot() *domain.Workflow {
	return &domain.Workflow{
		ID:                "wf-1",
		ProjectID:         "project-1",
		Name:              "Build",
		Slug:              "build",
		Description:       "Build workflow",
		Tags:              map[string]string{"team": "platform"},
		Version:           3,
		VersionID:         "version-1",
		TimeoutSecs:       300,
		MaxConcurrentRuns: 4,
		MaxParallelSteps:  2,
	}
}

func TestGetOrCreateWorkflowSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("returns existing snapshot for matching definition hash", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		queryRows := 0
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				queryRows++
				require.Contains(t, sql, "definition_hash")
				require.Equal(t, []any{"wf-1", "version-1", args[2], "project-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					fillWorkflowSnapshotDest(dest, "snapshot-existing", createdAt)
					return nil
				}}
			},
		}

		got, err := New(db).GetOrCreateWorkflowSnapshot(context.Background(), testWorkflowForSnapshot(), nil)
		require.NoError(t, err)
		require.Equal(t, "snapshot-existing", got.ID)
		require.Equal(t, createdAt, got.CreatedAt)
		require.JSONEq(t, `{"workflow":{"id":"wf-1"},"steps":[]}`, string(got.Definition))
		require.Equal(t, 1, queryRows)
	})

	t.Run("inserts when definition hash has no match", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		var insertArgs []any
		queryRows := 0
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				queryRows++
				if strings.Contains(sql, "INSERT INTO workflow_snapshots") {
					insertArgs = append([]any(nil), args...)
					return &mockRow{scanFn: func(dest ...any) error {
						*(dest[0].(*string)) = "snapshot-inserted"
						*(dest[1].(*time.Time)) = createdAt
						return nil
					}}
				}
				return &mockRow{scanFn: func(...any) error {
					return pgx.ErrNoRows
				}}
			},
		}

		got, err := New(db).GetOrCreateWorkflowSnapshot(context.Background(), testWorkflowForSnapshot(), []domain.WorkflowStep{
			{ID: "step-1", WorkflowID: "wf-1", StepRef: "build", StepType: domain.WorkflowStepTypeJob},
		})
		require.NoError(t, err)
		require.Equal(t, "snapshot-inserted", got.ID)
		require.Equal(t, createdAt, got.CreatedAt)
		require.Equal(t, 2, queryRows)
		require.Len(t, insertArgs, 7)
		require.Equal(t, "wf-1", insertArgs[1])
		require.Equal(t, "project-1", insertArgs[2])
		require.Len(t, insertArgs[6].(string), 64)

		parsed, err := ParseSnapshotDefinition(got.Definition)
		require.NoError(t, err)
		require.Len(t, parsed.Steps, 1)
		require.Equal(t, "build", parsed.Steps[0].StepRef)
	})

	t.Run("wraps insert errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
				if strings.Contains(sql, "INSERT INTO workflow_snapshots") {
					return &mockRow{scanFn: func(...any) error {
						return errors.New("insert failed")
					}}
				}
				return &mockRow{scanFn: func(...any) error {
					return pgx.ErrNoRows
				}}
			},
		}

		_, err := New(db).GetOrCreateWorkflowSnapshot(context.Background(), testWorkflowForSnapshot(), nil)
		require.ErrorContains(t, err, "insert workflow snapshot")
		require.ErrorContains(t, err, "insert failed")
	})
}

func TestGetWorkflowSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("returns tenant-scoped snapshot", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
				require.Contains(t, sql, "WHERE id = $1 AND project_id = $2")
				require.Equal(t, []any{"snapshot-1", "project-1"}, args)
				return &mockRow{scanFn: func(dest ...any) error {
					fillWorkflowSnapshotDest(dest, "snapshot-1", createdAt)
					return nil
				}}
			},
		}

		got, err := New(db).GetWorkflowSnapshot(context.Background(), "project-1", "snapshot-1")
		require.NoError(t, err)
		require.Equal(t, "snapshot-1", got.ID)
		require.Equal(t, "wf-1", got.WorkflowID)
		require.JSONEq(t, `{"workflow":{"id":"wf-1"},"steps":[]}`, string(got.Definition))
	})

	t.Run("maps missing rows to nil snapshot", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return pgx.ErrNoRows
				}}
			},
		}

		got, err := New(db).GetWorkflowSnapshot(context.Background(), "project-1", "missing")
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("wraps scan errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryRowFn: func(context.Context, string, ...any) pgx.Row {
				return &mockRow{scanFn: func(...any) error {
					return errors.New("scan failed")
				}}
			},
		}

		_, err := New(db).GetWorkflowSnapshot(context.Background(), "project-1", "snapshot-1")
		require.ErrorContains(t, err, "get workflow snapshot")
		require.ErrorContains(t, err, "scan failed")
	})
}

func TestListWorkflowSnapshotsByWorkflow(t *testing.T) {
	t.Parallel()

	t.Run("returns snapshots newest first from rows", func(t *testing.T) {
		t.Parallel()

		createdAt := time.Now().UTC()
		db := &mockDBTX{
			queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				require.Contains(t, sql, "ORDER BY created_at DESC")
				require.Equal(t, []any{"wf-1", "project-1", 2}, args)
				return &mockRows{scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						fillWorkflowSnapshotDest(dest, "snapshot-2", createdAt)
						return nil
					},
					func(dest ...any) error {
						fillWorkflowSnapshotDest(dest, "snapshot-1", createdAt.Add(-time.Minute))
						return nil
					},
				}}, nil
			},
		}

		got, err := New(db).ListWorkflowSnapshotsByWorkflow(context.Background(), "project-1", "wf-1", 2)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, "snapshot-2", got[0].ID)
		require.Equal(t, "snapshot-1", got[1].ID)
		require.JSONEq(t, `{"workflow":{"id":"wf-1"},"steps":[]}`, string(got[0].Definition))
	})

	t.Run("wraps query errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return nil, errors.New("query failed")
			},
		}

		_, err := New(db).ListWorkflowSnapshotsByWorkflow(context.Background(), "project-1", "wf-1", 2)
		require.ErrorContains(t, err, "list workflow snapshots")
		require.ErrorContains(t, err, "query failed")
	})

	t.Run("wraps row scan errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{scanFns: []func(dest ...any) error{
					func(...any) error {
						return errors.New("row failed")
					},
				}}, nil
			},
		}

		_, err := New(db).ListWorkflowSnapshotsByWorkflow(context.Background(), "project-1", "wf-1", 2)
		require.ErrorContains(t, err, "scan workflow snapshot")
		require.ErrorContains(t, err, "row failed")
	})

	t.Run("returns row iteration errors", func(t *testing.T) {
		t.Parallel()

		db := &mockDBTX{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &mockRows{err: errors.New("rows failed")}, nil
			},
		}

		_, err := New(db).ListWorkflowSnapshotsByWorkflow(context.Background(), "project-1", "wf-1", 2)
		require.ErrorContains(t, err, "rows failed")
	})
}

func TestParseSnapshotDefinition_RoundTrip(t *testing.T) {
	steps := []domain.WorkflowStep{
		{
			ID:                    "step-1",
			WorkflowID:            "wf-1",
			JobID:                 "job-1",
			StepRef:               "build",
			DependsOn:             []string{},
			Condition:             json.RawMessage(`{"all_of":["deploy"]}`),
			OnFailure:             domain.FailWorkflow,
			Payload:               json.RawMessage(`{"key":"value"}`),
			StepType:              domain.WorkflowStepTypeJob,
			ApprovalTimeoutSecs:   600,
			ApprovalApprovers:     []string{"alice", "bob"},
			RetryMaxAttempts:      3,
			RetryBackoff:          domain.RetryBackoffExponential,
			RetryInitialDelaySecs: 5,
			RetryMaxDelaySecs:     60,
			TimeoutSecsOverride:   120,
			OutputTransform:       "$.result",
			SubWorkflowID:         "",
			MaxNestingDepth:       5,
			EventKey:              "my-event",
			EventTimeoutSecs:      3600,
			EventNotifyURL:        "https://example.com/notify",
			SleepDurationSecs:     30,
			EventEmitKey:          "emit-key",
			ConcurrencyKey:        "ck-1",
			ResourceClass:         "medium",
		},
		{
			ID:         "step-2",
			WorkflowID: "wf-1",
			JobID:      "job-2",
			StepRef:    "deploy",
			DependsOn:  []string{"build"},
			OnFailure:  domain.Continue,
			StepType:   domain.WorkflowStepTypeApproval,
		},
		{
			ID:            "step-3",
			WorkflowID:    "wf-1",
			StepRef:       "wait",
			StepType:      domain.WorkflowStepTypeWaitForEvent,
			EventKey:      "deploy-done",
			DependsOn:     []string{"deploy"},
			OnFailure:     domain.SkipDependents,
			ResourceClass: "small",
		},
		{
			ID:                "step-4",
			WorkflowID:        "wf-1",
			StepRef:           "sleep-step",
			StepType:          domain.WorkflowStepTypeSleep,
			SleepDurationSecs: 60,
			DependsOn:         []string{"wait"},
			OnFailure:         domain.FailWorkflow,
		},
		{
			ID:            "step-5",
			WorkflowID:    "wf-1",
			StepRef:       "sub-wf",
			StepType:      domain.WorkflowStepTypeSubWorkflow,
			SubWorkflowID: "child-wf-1",
			DependsOn:     []string{"sleep-step"},
			OnFailure:     domain.FailWorkflow,
		},
	}

	def := domain.WorkflowSnapshotDefinition{
		Workflow: domain.WorkflowSnapshotMeta{
			ID:                "wf-1",
			ProjectID:         "proj-1",
			Name:              "My Workflow",
			Slug:              "my-workflow",
			Description:       "A test workflow",
			Tags:              map[string]string{"team": "platform"},
			Version:           3,
			VersionID:         "vid-abc",
			TimeoutSecs:       3600,
			MaxConcurrentRuns: 5,
			MaxParallelSteps:  3,
		},
		Steps: steps,
	}

	// Serialize
	data, err := json.Marshal(def)
	require.NoError(t, err)

	// Deserialize
	parsed, err := ParseSnapshotDefinition(json.RawMessage(data))
	require.NoError(t, err)
	assert.Equal(t, "wf-1",
		parsed.Workflow.
			ID)
	assert.Equal(t, "My Workflow",
		parsed.
			Workflow.Name)
	assert.Equal(t, 3, parsed.
		Workflow.
		Version)
	assert.Equal(t, "platform",
		parsed.
			Workflow.Tags["team"])
	require.Len(t, parsed.
		Steps, 5)

	stepTypes := map[string]domain.WorkflowStepType{
		"build":      domain.WorkflowStepTypeJob,
		"deploy":     domain.WorkflowStepTypeApproval,
		"wait":       domain.WorkflowStepTypeWaitForEvent,
		"sleep-step": domain.WorkflowStepTypeSleep,
		"sub-wf":     domain.WorkflowStepTypeSubWorkflow,
	}
	for _, step := range parsed.Steps {
		expected, ok := stepTypes[step.StepRef]
		if !ok {
			assert.Failf(t, "test failure",

				"unexpected step ref %q", step.StepRef)
			continue
		}
		assert.Equal(t, expected,
			step.StepType,
		)
	}

	s := parsed.Steps[0]
	assert.Equal(t, "job-1",
		s.JobID,
	)
	assert.Equal(t, 3, s.
		RetryMaxAttempts,
	)
	assert.Equal(t, domain.
		RetryBackoffExponential,

		s.RetryBackoff,
	)
	assert.Equal(t, 5, s.
		RetryInitialDelaySecs,
	)
	assert.Equal(t, 120,
		s.TimeoutSecsOverride,
	)
	assert.Equal(t, "$.result",
		s.OutputTransform,
	)
	assert.Equal(t, "my-event",
		s.EventKey,
	)
	assert.Equal(t, "https://example.com/notify",

		s.EventNotifyURL,
	)
	assert.Equal(t, "ck-1",
		s.ConcurrencyKey,
	)
	assert.Equal(t, "medium",
		s.ResourceClass,
	)
	assert.Equal(t, domain.
		FailWorkflow,

		s.OnFailure)
	assert.JSONEq(t, `{"all_of":["deploy"]}`,

		string(s.Condition))
	assert.JSONEq(t, `{"key":"value"}`,

		string(s.Payload))
	assert.Equal(t, 600,
		s.ApprovalTimeoutSecs,
	)
	assert.False(t, len(s.
		ApprovalApprovers,
	) != 2 || s.ApprovalApprovers[0] != "alice")
	assert.Equal(t, 30, s.
		SleepDurationSecs,
	)
	assert.Equal(t, "emit-key",
		s.EventEmitKey,
	)
}

func TestParseSnapshotDefinition_ComplexConditions(t *testing.T) {
	def := domain.WorkflowSnapshotDefinition{
		Steps: []domain.WorkflowStep{
			{
				StepRef:   "step-nested",
				Condition: json.RawMessage(`{"any_of":[{"all_of":["a","b"]},{"none_of":["c"]}]}`),
			},
		},
	}

	data, _ := json.Marshal(def)
	parsed, err := ParseSnapshotDefinition(data)
	require.NoError(t, err)
	assert.JSONEq(t, `{"any_of":[{"all_of":["a","b"]},{"none_of":["c"]}]}`,

		string(parsed.Steps[0].Condition))
}

func TestParseSnapshotDefinition_EmptyOptionalFields(t *testing.T) {
	def := domain.WorkflowSnapshotDefinition{
		Steps: []domain.WorkflowStep{
			{
				StepRef:   "minimal",
				StepType:  domain.WorkflowStepTypeJob,
				OnFailure: domain.FailWorkflow,
			},
		},
	}

	data, _ := json.Marshal(def)
	parsed, err := ParseSnapshotDefinition(data)
	require.NoError(t, err)

	s := parsed.Steps[0]
	assert.Empty(t, s.
		OutputTransform,
	)
	assert.Empty(t, s.
		EventKey)
	assert.Empty(t, s.
		SubWorkflowID,
	)
	assert.Nil(t, s.Condition)
	assert.Nil(t, s.Payload)
}

func TestParseSnapshotDefinition_AllRetryFields(t *testing.T) {
	def := domain.WorkflowSnapshotDefinition{
		Steps: []domain.WorkflowStep{
			{
				StepRef:               "retry-step",
				RetryMaxAttempts:      5,
				RetryBackoff:          domain.RetryBackoffFixed,
				RetryInitialDelaySecs: 10,
				RetryMaxDelaySecs:     120,
			},
		},
	}

	data, _ := json.Marshal(def)
	parsed, err := ParseSnapshotDefinition(data)
	require.NoError(t, err)

	s := parsed.Steps[0]
	assert.Equal(t, 5, s.
		RetryMaxAttempts,
	)
	assert.Equal(t, domain.
		RetryBackoffFixed,

		s.RetryBackoff)
	assert.Equal(t, 10, s.
		RetryInitialDelaySecs,
	)
	assert.Equal(t, 120,
		s.RetryMaxDelaySecs,
	)
}

func TestParseSnapshotDefinition_ExhaustiveFieldCheck(t *testing.T) {
	// Create a step with ALL fields populated to ensure nothing is lost.
	step := domain.WorkflowStep{
		ID:                    "ws-id-1",
		WorkflowID:            "wf-id-1",
		JobID:                 "job-id-1",
		StepRef:               "exhaustive",
		DependsOn:             []string{"a", "b", "c"},
		Condition:             json.RawMessage(`{"op":"eq","field":"status","value":"ok"}`),
		OnFailure:             domain.SkipDependents,
		Payload:               json.RawMessage(`{"x":1,"nested":{"y":2}}`),
		StepType:              domain.WorkflowStepTypeJob,
		ApprovalTimeoutSecs:   900,
		ApprovalApprovers:     []string{"admin"},
		RetryMaxAttempts:      7,
		RetryBackoff:          domain.RetryBackoffExponential,
		RetryInitialDelaySecs: 2,
		RetryMaxDelaySecs:     300,
		TimeoutSecsOverride:   600,
		OutputTransform:       "$.data.result",
		SubWorkflowID:         "sub-wf-id",
		MaxNestingDepth:       3,
		EventKey:              "evt-key-123",
		EventTimeoutSecs:      7200,
		EventNotifyURL:        "https://hooks.example.com",
		SleepDurationSecs:     45,
		EventEmitKey:          "emit-done",
		ConcurrencyKey:        "project:deploy",
		ResourceClass:         "large",
	}

	def := domain.WorkflowSnapshotDefinition{
		Workflow: domain.WorkflowSnapshotMeta{ID: "wf-id-1"},
		Steps:    []domain.WorkflowStep{step},
	}

	data, _ := json.Marshal(def)
	parsed, err := ParseSnapshotDefinition(data)
	require.NoError(t, err)

	got := parsed.Steps[0]

	// Check every field
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"ID", got.ID, step.ID},
		{"WorkflowID", got.WorkflowID, step.WorkflowID},
		{"JobID", got.JobID, step.JobID},
		{"StepRef", got.StepRef, step.StepRef},
		{"DependsOn_len", len(got.DependsOn), 3},
		{"OnFailure", string(got.OnFailure), string(step.OnFailure)},
		{"StepType", string(got.StepType), string(step.StepType)},
		{"ApprovalTimeoutSecs", got.ApprovalTimeoutSecs, step.ApprovalTimeoutSecs},
		{"ApprovalApprovers_len", len(got.ApprovalApprovers), 1},
		{"RetryMaxAttempts", got.RetryMaxAttempts, step.RetryMaxAttempts},
		{"RetryBackoff", string(got.RetryBackoff), string(step.RetryBackoff)},
		{"RetryInitialDelaySecs", got.RetryInitialDelaySecs, step.RetryInitialDelaySecs},
		{"RetryMaxDelaySecs", got.RetryMaxDelaySecs, step.RetryMaxDelaySecs},
		{"TimeoutSecsOverride", got.TimeoutSecsOverride, step.TimeoutSecsOverride},
		{"OutputTransform", got.OutputTransform, step.OutputTransform},
		{"SubWorkflowID", got.SubWorkflowID, step.SubWorkflowID},
		{"MaxNestingDepth", got.MaxNestingDepth, step.MaxNestingDepth},
		{"EventKey", got.EventKey, step.EventKey},
		{"EventTimeoutSecs", got.EventTimeoutSecs, step.EventTimeoutSecs},
		{"EventNotifyURL", got.EventNotifyURL, step.EventNotifyURL},
		{"SleepDurationSecs", got.SleepDurationSecs, step.SleepDurationSecs},
		{"EventEmitKey", got.EventEmitKey, step.EventEmitKey},
		{"ConcurrencyKey", got.ConcurrencyKey, step.ConcurrencyKey},
		{"ResourceClass", got.ResourceClass, step.ResourceClass},
	}

	for _, c := range checks {
		assert.Equal(t, c.want,
			c.got)
	}
}

func TestParseSnapshotDefinition_EmptyDefinition(t *testing.T) {
	t.Parallel()
	_, err := ParseSnapshotDefinition(nil)
	require.Error(t, err)

	_, err = ParseSnapshotDefinition(json.RawMessage(``))
	assert.Error(t, err)
}

func TestParseSnapshotDefinition_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := ParseSnapshotDefinition(json.RawMessage(`{broken`))
	assert.Error(t, err)
}

func TestParseSnapshotDefinition_DuplicateStepRefs(t *testing.T) {
	t.Parallel()
	def := domain.WorkflowSnapshotDefinition{
		Steps: []domain.WorkflowStep{
			{StepRef: "build", StepType: domain.WorkflowStepTypeJob},
			{StepRef: "build", StepType: domain.WorkflowStepTypeJob}, // duplicate
		},
	}
	data, _ := json.Marshal(def)
	_, err := ParseSnapshotDefinition(data)
	assert.Error(t, err)
}

func TestParseSnapshotDefinition_ZeroSteps(t *testing.T) {
	t.Parallel()
	def := domain.WorkflowSnapshotDefinition{
		Workflow: domain.WorkflowSnapshotMeta{ID: "wf-1"},
		Steps:    []domain.WorkflowStep{},
	}
	data, _ := json.Marshal(def)
	// Zero steps is valid — a workflow can have no steps at trigger time
	// (e.g., all steps disabled via overrides).
	parsed, err := ParseSnapshotDefinition(data)
	require.NoError(t, err)
	assert.Empty(t, parsed.
		Steps)
}

func TestParseSnapshotDefinition_UniqueStepRefs_Pass(t *testing.T) {
	t.Parallel()
	def := domain.WorkflowSnapshotDefinition{
		Steps: []domain.WorkflowStep{
			{StepRef: "a"},
			{StepRef: "b"},
			{StepRef: "c"},
		},
	}
	data, _ := json.Marshal(def)
	parsed, err := ParseSnapshotDefinition(data)
	require.NoError(t, err)
	assert.Len(t, parsed.
		Steps, 3)
}
