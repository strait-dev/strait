package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditDeadletterScopedIDValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		id        string
		projectID string
		want      bool
	}{
		{name: "id and project", id: "dlq-1", projectID: "proj-1", want: true},
		{name: "missing id", id: "", projectID: "proj-1", want: false},
		{name: "missing project", id: "dlq-1", projectID: "", want: false},
		{name: "missing both", id: "", projectID: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, auditDeadletterScopedIDValid(tt.id, tt.projectID))
		})
	}
}

func TestAuditDeadletterReplayIDsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		id         string
		projectID  string
		newEventID string
		want       bool
	}{
		{name: "all ids", id: "dlq-1", projectID: "proj-1", newEventID: "audit-1", want: true},
		{name: "missing deadletter id", id: "", projectID: "proj-1", newEventID: "audit-1", want: false},
		{name: "missing project", id: "dlq-1", projectID: "", newEventID: "audit-1", want: false},
		{name: "missing new event", id: "dlq-1", projectID: "proj-1", newEventID: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, auditDeadletterReplayIDsValid(tt.id, tt.projectID, tt.newEventID))
		})
	}
}
