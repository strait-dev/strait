package domain

import (
	"fmt"
	"strings"
	"testing"
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
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is not a string: %v", r)
		}
		if !strings.Contains(msg, "injected failure") {
			t.Fatalf("panic message %q does not contain 'injected failure'", msg)
		}
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
		if got := tc.status.IsValid(); got != tc.want {
			t.Errorf("DeploymentVersionStatus(%q).IsValid() = %v, want %v", tc.status, got, tc.want)
		}
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
		if got := tc.policy.IsValid(); got != tc.want {
			t.Errorf("CronOverlapPolicy(%q).IsValid() = %v, want %v", tc.policy, got, tc.want)
		}
	}
}
