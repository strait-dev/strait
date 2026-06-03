// NOTE: This file contains Huma operation registrations for non-TypedHandler
// routes only (health checks, SSE streams). All TypedHandler operations are
// registered via registerAllTypedOps in huma_registry.go.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// registerHumaOperations registers Huma operations for raw (non-TypedHandler)
// routes. TypedHandler operations are registered separately in huma_registry.go.
func (s *Server) registerHumaOperations(api huma.API) {
	s.registerHealthOps(api)
	s.registerStreamOps(api)
}

// registerHealthOps registers health check operations.

type HealthCheckOutput struct {
	Body struct {
		Status  string `json:"status" example:"ok" doc:"Service health status"`
		Edition string `json:"edition,omitempty" example:"cloud" doc:"Service edition"`
	}
}

func (s *Server) registerHealthOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "health-check",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Health check",
		Description: "Returns the service health status.",
		Tags:        []string{"Health"},
	}, func(_ context.Context, _ *struct{}) (*HealthCheckOutput, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "health-ready",
		Method:      http.MethodGet,
		Path:        "/health/ready",
		Summary:     "Readiness check",
		Description: "Returns 200 when the service is ready to accept traffic.",
		Tags:        []string{"Health"},
	}, func(_ context.Context, _ *struct{}) (*HealthCheckOutput, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}

// registerStreamOps registers SSE stream and raw handler operations that
// cannot use RegisterTypedOp because they are not TypedHandler routes.
func (s *Server) registerStreamOps(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "stream-event-trigger",
		Method:      http.MethodGet,
		Path:        "/v1/events/{eventKey}/stream",
		Summary:     "Stream event trigger updates",
		Description: "Opens an SSE stream for real-time updates on an event trigger.",
		Tags:        []string{"Events"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		EventKey string `path:"eventKey" doc:"Event key" example:"order.completed.12345"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "stream-run",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/stream",
		Summary:     "Stream run updates",
		Description: "Opens an SSE stream for real-time updates on a run's execution.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-run-chunk-stream",
		Method:      http.MethodGet,
		Path:        "/v1/runs/{runID}/stream/chunks",
		Summary:     "Get run stream chunks",
		Description: "Returns stored run streaming chunks for a run.",
		Tags:        []string{"Runs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{401, 404, 500},
	}, func(_ context.Context, _ *struct {
		RunID string `path:"runID" doc:"Run ID" example:"run_01HX8BQNP4"`
	}) (*struct{ Body any }, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})

	huma.Register(api, huma.Operation{
		OperationID: "stream-project-activity",
		Method:      http.MethodGet,
		Path:        "/v1/projects/{projectID}/activity/stream",
		Summary:     "Stream project activity",
		Description: "Opens an SSE stream for real-time updates on all project activity including job runs, workflow runs, and event triggers.",
		Tags:        []string{"Projects"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{400, 401, 500, 503},
	}, func(_ context.Context, _ *struct {
		ProjectID string `path:"projectID" doc:"Project ID" example:"proj_01HX8BQNP4"`
	}) (*struct{}, error) {
		return nil, nil //nolint:nilnil // doc-only stub
	})
}
