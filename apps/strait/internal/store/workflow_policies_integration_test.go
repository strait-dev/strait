//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestUpsertWorkflowPolicy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	policy := &domain.WorkflowPolicy{
		ProjectID:                "project-workflow-policy",
		MaxFanOut:                10,
		MaxDepth:                 5,
		ForbiddenStepTypes:       []string{"dangerous"},
		RequireApprovalForDeploy: true,
	}
	require.NoError(t, q.UpsertWorkflowPolicy(ctx,
		policy))
	require.NotEqual(t, "",

		policy.ID,
	)
	require.False(t, policy.
		CreatedAt.
		IsZero())

	got, err := q.GetWorkflowPolicyByProject(ctx, "project-workflow-policy")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.EqualValues(t, 10, got.
		MaxFanOut,
	)
	require.EqualValues(t, 5, got.
		MaxDepth)
	require.True(t, got.RequireApprovalForDeploy)
	require.False(t, len(got.
		ForbiddenStepTypes,
	) !=
		1 || got.
		ForbiddenStepTypes[0] !=
		"dangerous",
	)

}

func TestUpsertWorkflowPolicy_Update(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	policy := &domain.WorkflowPolicy{
		ProjectID: "project-workflow-policy-update",
		MaxFanOut: 5,
		MaxDepth:  3,
	}
	require.NoError(t, q.UpsertWorkflowPolicy(ctx,
		policy))

	initialID := policy.ID
	initialUpdatedAt := policy.UpdatedAt
	var xminBeforeNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM workflow_policies
		WHERE project_id = $1`,

		"project-workflow-policy-update",
	).Scan(&xminBeforeNoop))

	same := &domain.WorkflowPolicy{
		ProjectID: "project-workflow-policy-update",
		MaxFanOut: 5,
		MaxDepth:  3,
	}
	require.NoError(t, q.UpsertWorkflowPolicy(ctx,
		same))

	var xminAfterNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM workflow_policies
		WHERE project_id = $1`,

		"project-workflow-policy-update",
	).Scan(&xminAfterNoop))
	require.Equal(t, xminBeforeNoop,

		xminAfterNoop,
	)
	require.Equal(t, initialID,

		same.
			ID)
	require.True(t, same.UpdatedAt.
		Equal(initialUpdatedAt))

	// Update.
	updated := &domain.WorkflowPolicy{
		ProjectID: "project-workflow-policy-update",
		MaxFanOut: 20,
		MaxDepth:  10,
	}
	require.NoError(t, q.UpsertWorkflowPolicy(ctx,
		updated),
	)

	got, err := q.GetWorkflowPolicyByProject(ctx, "project-workflow-policy-update")
	require.NoError(t, err)
	require.EqualValues(t, 20, got.
		MaxFanOut,
	)
	require.EqualValues(t, 10, got.
		MaxDepth,
	)

}

func TestGetWorkflowPolicyByProject_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.GetWorkflowPolicyByProject(ctx, "nonexistent-project")
	require.NoError(t, err)
	require.Nil(t, got)

}
