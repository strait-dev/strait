package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyAPIKeyLifetimePolicy(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)

	got, err := ApplyAPIKeyLifetimePolicy(now, nil, 7)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, now.Add(7*24*time.Hour), *got)

	requested := now.Add(3 * 24 * time.Hour)
	got, err = ApplyAPIKeyLifetimePolicy(now, &requested, 7)
	require.NoError(t, err)
	assert.Same(t, &requested, got)

	tooLate := now.Add(8 * 24 * time.Hour)
	got, err = ApplyAPIKeyLifetimePolicy(now, &tooLate, 7)
	require.Error(t, err)
	assert.Nil(t, got)

	got, err = ApplyAPIKeyLifetimePolicy(now, nil, 0)
	require.Error(t, err)
	assert.Nil(t, got)
}

func TestApplyAPIKeyLifetimePolicyCapsConfiguredMaximum(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	got, err := ApplyAPIKeyLifetimePolicy(now, nil, MaxAPIKeyDurationDays+1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, now.Add(time.Duration(MaxAPIKeyDurationDays)*24*time.Hour), *got)
}

func TestAllRegionsSortedAndLabeled(t *testing.T) {
	t.Parallel()

	regions := AllRegions()
	require.NotEmpty(t, regions)

	for i := 1; i < len(regions); i++ {
		assert.Less(t, regions[i-1].Code, regions[i].Code)
	}

	first := regions[0]
	assert.True(t, IsValidRegion(first.Code))
	assert.Equal(t, first.Label, RegionLabel(first.Code))
	assert.Equal(t, "unknown", RegionLabel("unknown"))
}

func TestCloneWorkflowStepsCopiesMutableFields(t *testing.T) {
	t.Parallel()

	assert.Nil(t, CloneWorkflowSteps(nil))

	steps := []WorkflowStep{{
		ID:                 "step-1",
		DependsOn:          []string{"a"},
		Condition:          []byte(`{"ok":true}`),
		Payload:            []byte(`{"value":1}`),
		ApprovalApprovers:  []string{"ops@example.com"},
		StageNotifications: []byte(`{"on_failure":true}`),
	}}

	cloned := CloneWorkflowSteps(steps)
	require.Len(t, cloned, 1)
	require.Equal(t, steps, cloned)

	cloned[0].DependsOn[0] = "changed"
	cloned[0].Condition[0] = '['
	cloned[0].Payload[0] = '['
	cloned[0].ApprovalApprovers[0] = "security@example.com"
	cloned[0].StageNotifications[0] = '['

	assert.Equal(t, "a", steps[0].DependsOn[0])
	assert.Equal(t, byte('{'), steps[0].Condition[0])
	assert.Equal(t, byte('{'), steps[0].Payload[0])
	assert.Equal(t, "ops@example.com", steps[0].ApprovalApprovers[0])
	assert.Equal(t, byte('{'), steps[0].StageNotifications[0])
}

func TestWorkflowStepTimeoutBounds(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 60, DefaultSleepDurationSecs)
	assert.Equal(t, 30*24*3600, MaxSleepDurationSecs)
	assert.Equal(t, 30*24*3600, MaxEventTimeoutSecs)
}
