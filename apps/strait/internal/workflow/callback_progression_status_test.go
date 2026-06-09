package workflow

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestIsPendingOrWaitingStepStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status domain.StepRunStatus
		want   bool
	}{
		{name: "pending", status: domain.StepPending, want: true},
		{name: "waiting", status: domain.StepWaiting, want: true},
		{name: "running", status: domain.StepRunning},
		{name: "completed", status: domain.StepCompleted},
		{name: "failed", status: domain.StepFailed},
		{name: "skipped", status: domain.StepSkipped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, isPendingOrWaitingStepStatus(tt.status))
		})
	}
}
