package domain

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVersionID_PanicsOnGenerateError(t *testing.T) {
	// Not parallel: modifies package-level nanoidGenerate.
	orig := nanoidGenerate
	t.Cleanup(func() { nanoidGenerate = orig })

	nanoidGenerate = func(alphabet string, size int) (string, error) {
		return "", fmt.Errorf("injected failure")
	}

	defer func() {
		r := recover()
		require.NotNil(t, r)

		msg, ok := r.(string)
		require.True(t, ok)
		require.Contains(t, msg, "injected failure")
	}()

	NewVersionID()
}

func TestDeploymentVersionStatus_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status DeploymentVersionStatus
		want   bool
	}{
		{DeploymentVersionStatusDraft, true},
		{DeploymentVersionStatusFinalized, true},
		{DeploymentVersionStatusPromoted, true},
		{DeploymentVersionStatus(""), false},
		{DeploymentVersionStatus("archived"), false},
		{DeploymentVersionStatus("DRAFT"), false},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.status.IsValid())
	}
}

func TestCronOverlapPolicy_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		policy CronOverlapPolicy
		want   bool
	}{
		{OverlapPolicyAllow, true},
		{OverlapPolicySkip, true},
		{OverlapPolicyCancelRunning, true},
		{CronOverlapPolicy(""), false},
		{CronOverlapPolicy("queue"), false},
		{CronOverlapPolicy("ALLOW"), false},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.policy.IsValid())
	}
}
