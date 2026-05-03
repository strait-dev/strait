package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestCodeDeploymentRoutesRemoved asserts that the code-deployment HTTP routes
// are no longer registered in the chi router. It walks all registered patterns
// and fails if any deployment-under-jobs pattern is found.
func TestCodeDeploymentRoutesRemoved(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	// Patterns we expect to be absent.
	forbidden := []string{
		"/v1/jobs/{jobID}/deployments",
		"/v1/jobs/{jobID}/deployments/{deploymentID}",
		"/v1/jobs/{jobID}/deployments/{deploymentID}/confirm",
		"/v1/jobs/{jobID}/deployments/{deploymentID}/rollback",
		"/v1/jobs/{jobID}/deployments/{deploymentID}/logs",
		"/internal/admin/orgs/{orgID}/deployments",
	}

	var registered []string
	if err := chi.Walk(srv.router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		registered = append(registered, route)
		return nil
	}); err != nil {
		t.Fatalf("chi.Walk: %v", err)
	}

	for _, want := range forbidden {
		for _, got := range registered {
			if strings.Contains(got, want) {
				t.Errorf("route %q should have been removed but is still registered as %q", want, got)
			}
		}
	}
}
