package api

import (
	"net/http"
	"time"

	"orchestrator/internal/domain"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/riandyrn/otelchi"
)

func (s *Server) routes() chi.Router {
	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.config.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Secret", "X-Idempotency-Key", "Idempotency-Key"},
		ExposedHeaders:   []string{"Link", "X-Request-Id", "X-API-Version"},
		AllowCredentials: s.config.CORSAllowCredentials,
		MaxAge:           300,
	}))

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(otelchi.Middleware("orchestrator", otelchi.WithChiRoutes(r)))
	r.Use(s.requestLogger)
	r.Use(chimw.Recoverer)
	r.Use(apiVersionHeader)
	requestTimeout := s.config.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = 30 * time.Second
	}
	if s.config.RateLimitRequests > 0 {
		r.Use(httprate.LimitByIP(s.config.RateLimitRequests, s.config.RateLimitWindow))
	}

	triggerRateLimitRequests := s.config.TriggerRateLimitRequests
	if triggerRateLimitRequests <= 0 {
		triggerRateLimitRequests = 10
	}
	triggerRateLimitWindow := s.config.TriggerRateLimitWindow
	if triggerRateLimitWindow <= 0 {
		triggerRateLimitWindow = time.Minute
	}

	// rateLimitEnabled controls whether per-route rate limiters are applied.
	// When the global rate limit is disabled (RateLimitRequests=0), per-route
	// rate limits are also skipped. This allows tests to run without hitting 429s.
	rateLimitEnabled := s.config.RateLimitRequests > 0
	// rateLimit returns a rate limiting middleware if enabled, otherwise a no-op.
	rateLimit := func(requests int, window time.Duration) func(http.Handler) http.Handler {
		if !rateLimitEnabled {
			return func(next http.Handler) http.Handler { return next }
		}
		return httprate.LimitByIP(requests, window)
	}

	r.Get("/health", s.handleHealth)
	r.Get("/health/ready", s.handleHealthReady)
	if s.metricsHandler != nil {
		r.Handle("/metrics", s.metricsHandler)
	}

	r.Route("/v1", func(r chi.Router) {
		r.Use(s.apiKeyOrSecretAuth)
		r.Use(chimw.Timeout(requestTimeout))
		r.Route("/secrets", func(r chi.Router) {
			r.With(requireScope(domain.ScopeSecretsWrite), rateLimit(20, time.Minute)).Post("/", s.handleCreateSecret)
			r.With(requireScope(domain.ScopeSecretsRead)).Get("/", s.handleListSecrets)
			r.With(requireScope(domain.ScopeSecretsWrite)).Delete("/{secretID}", s.handleDeleteSecret)
		})

		r.Route("/jobs", func(r chi.Router) {
			r.With(requireScope(domain.ScopeJobsWrite), rateLimit(30, time.Minute)).Post("/", s.handleCreateJob)
			r.With(requireScope(domain.ScopeJobsRead)).Get("/", s.handleListJobs)
			r.With(requireScope(domain.ScopeJobsWrite), rateLimit(10, time.Minute)).Post("/batch", s.handleBatchCreateJobs)
			r.With(requireScope(domain.ScopeJobsWrite)).Post("/batch-enable", s.handleBatchEnableJobs)
			r.With(requireScope(domain.ScopeJobsWrite)).Post("/batch-disable", s.handleBatchDisableJobs)

			r.Route("/{jobID}", func(r chi.Router) {
				r.With(requireScope(domain.ScopeJobsRead)).Get("/", s.handleGetJob)
				r.With(requireScope(domain.ScopeJobsWrite)).Patch("/", s.handleUpdateJob)
				r.With(requireScope(domain.ScopeJobsWrite)).Delete("/", s.handleDeleteJob)
				r.With(requireScope(domain.ScopeJobsTrigger), rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/trigger", s.handleTriggerJob)
				r.With(requireScope(domain.ScopeJobsTrigger), rateLimit(5, time.Minute)).Post("/trigger/bulk", s.handleBulkTriggerJob)
				r.With(requireScope(domain.ScopeJobsWrite)).Post("/dependencies", s.handleCreateJobDependency)
				r.With(requireScope(domain.ScopeJobsRead)).Get("/dependencies", s.handleListJobDependencies)
				r.With(requireScope(domain.ScopeJobsWrite)).Delete("/dependencies/{depID}", s.handleDeleteJobDependency)
				r.With(requireScope(domain.ScopeJobsRead)).Get("/versions", s.handleListJobVersions)
				r.With(requireScope(domain.ScopeJobsWrite)).Post("/clone", s.handleCloneJob)
				r.With(requireScope(domain.ScopeJobsRead)).Get("/health", s.handleGetJobHealth)
			})
		})

		r.Route("/job-groups", func(r chi.Router) {
			r.With(requireScope(domain.ScopeJobsWrite)).Post("/", s.handleCreateJobGroup)
			r.With(requireScope(domain.ScopeJobsRead)).Get("/", s.handleListJobGroups)
			r.Route("/{groupID}", func(r chi.Router) {
				r.With(requireScope(domain.ScopeJobsRead)).Get("/", s.handleGetJobGroup)
				r.With(requireScope(domain.ScopeJobsWrite)).Patch("/", s.handleUpdateJobGroup)
				r.With(requireScope(domain.ScopeJobsWrite)).Delete("/", s.handleDeleteJobGroup)
				r.With(requireScope(domain.ScopeJobsRead)).Get("/jobs", s.handleListJobsByGroup)
			})
		})

		r.Route("/environments", func(r chi.Router) {
			r.With(requireScope(domain.ScopeJobsWrite)).Post("/", s.handleCreateEnvironment)
			r.With(requireScope(domain.ScopeJobsRead)).Get("/", s.handleListEnvironments)
			r.Route("/{envID}", func(r chi.Router) {
				r.With(requireScope(domain.ScopeJobsRead)).Get("/", s.handleGetEnvironment)
				r.With(requireScope(domain.ScopeJobsWrite)).Patch("/", s.handleUpdateEnvironment)
				r.With(requireScope(domain.ScopeJobsWrite)).Delete("/", s.handleDeleteEnvironment)
				r.With(requireScope(domain.ScopeJobsRead)).Get("/variables", s.handleGetResolvedVariables)
			})
		})

		r.Route("/runs", func(r chi.Router) {
			r.With(requireScope(domain.ScopeRunsRead)).Get("/", s.handleListRuns)
			r.With(requireScope(domain.ScopeRunsRead)).Get("/dlq", s.handleListDeadLetterRuns)
			r.With(requireScope(domain.ScopeRunsWrite)).Post("/bulk-cancel", s.handleBulkCancelRuns)
			r.Route("/{runID}", func(r chi.Router) {
				r.With(requireScope(domain.ScopeRunsRead)).Get("/", s.handleGetRun)
				r.With(requireScope(domain.ScopeRunsWrite)).Delete("/", s.handleCancelRun)
				r.With(requireScope(domain.ScopeRunsWrite)).Post("/replay", s.handleReplayRun)
				r.With(requireScope(domain.ScopeRunsWrite)).Post("/dlq-replay", s.handleReplayDeadLetterRun)
				r.With(requireScope(domain.ScopeRunsRead)).Get("/stream", s.handleRunStream)
				r.With(requireScope(domain.ScopeRunsRead)).Get("/children", s.handleListChildRuns)
				r.With(requireScope(domain.ScopeRunsRead)).Get("/events", s.handleListRunEvents)
				r.With(requireScope(domain.ScopeRunsRead)).Get("/checkpoints", s.handleListRunCheckpoints)
				r.With(requireScope(domain.ScopeRunsRead)).Get("/usage", s.handleListRunUsage)
				r.With(requireScope(domain.ScopeRunsRead)).Get("/tool-calls", s.handleListRunToolCalls)
				r.With(requireScope(domain.ScopeRunsRead)).Get("/outputs", s.handleListRunOutputs)
				r.With(requireScope(domain.ScopeRunsRead)).Get("/debug-bundle", s.handleGetDebugBundle)
				r.With(requireScope(domain.ScopeRunsWrite)).Post("/debug", s.handleSetDebugMode)
				r.With(requireScope(domain.ScopeRunsRead)).Get("/lineage", s.handleListRunLineage)
			})
		})

		r.With(requireScope(domain.ScopeRunsRead)).Get("/webhook-deliveries", s.handleListWebhookDeliveries)

		r.Route("/api-keys", func(r chi.Router) {
			r.With(requireScope(domain.ScopeAPIKeysManage), httprate.LimitByIP(10, time.Minute)).Post("/", s.handleCreateAPIKey)
			r.With(requireScope(domain.ScopeAPIKeysManage)).Get("/", s.handleListAPIKeys)
			r.With(requireScope(domain.ScopeAPIKeysManage)).Delete("/{keyID}", s.handleRevokeAPIKey)
		})

		r.With(requireScope(domain.ScopeStatsRead)).Get("/stats", s.handleStats)

		r.Route("/workflows", func(r chi.Router) {
			r.With(requireScope(domain.ScopeWorkflowsWrite)).Post("/", s.handleCreateWorkflow)
			r.With(requireScope(domain.ScopeWorkflowsRead)).Get("/", s.handleListWorkflows)
			r.Route("/{workflowID}", func(r chi.Router) {
				r.With(requireScope(domain.ScopeWorkflowsRead)).Get("/", s.handleGetWorkflow)
				r.With(requireScope(domain.ScopeWorkflowsWrite)).Patch("/", s.handleUpdateWorkflow)
				r.With(requireScope(domain.ScopeWorkflowsWrite)).Delete("/", s.handleDeleteWorkflow)
				r.With(requireScope(domain.ScopeWorkflowsRead)).Post("/dry-run", s.handleDryRunWorkflow)
				r.With(requireScope(domain.ScopeWorkflowsRead)).Get("/graph", s.handleWorkflowGraph)
				r.With(requireScope(domain.ScopeWorkflowsTrigger)).Post("/trigger", s.handleTriggerWorkflow)
				r.With(requireScope(domain.ScopeWorkflowsWrite)).Post("/clone", s.handleCloneWorkflow)
				r.With(requireScope(domain.ScopeWorkflowsRead)).Get("/runs", s.handleListWorkflowRuns)
			})
		})

		r.Route("/workflow-runs", func(r chi.Router) {
			r.With(requireScope(domain.ScopeWorkflowsRead)).Get("/", s.handleListWorkflowRunsByProject)
			r.Route("/{workflowRunID}", func(r chi.Router) {
				r.With(requireScope(domain.ScopeWorkflowsRead)).Get("/", s.handleGetWorkflowRun)
				r.With(requireScope(domain.ScopeWorkflowsWrite)).Delete("/", s.handleCancelWorkflowRun)
				r.With(requireScope(domain.ScopeWorkflowsWrite)).Post("/pause", s.handlePauseWorkflowRun)
				r.With(requireScope(domain.ScopeWorkflowsWrite)).Post("/resume", s.handleResumeWorkflowRun)
				r.With(requireScope(domain.ScopeWorkflowsRead)).Get("/labels", s.handleGetWorkflowRunLabels)
				r.With(requireScope(domain.ScopeWorkflowsRead)).Get("/steps", s.handleListWorkflowStepRuns)
				r.With(requireScope(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/approve", s.handleApproveWorkflowStep)
				r.With(requireScope(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/skip", s.handleSkipWorkflowStep)
				r.With(requireScope(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/force-complete", s.handleForceCompleteWorkflowStep)
				r.With(requireScope(domain.ScopeWorkflowsTrigger)).Post("/retry", s.handleRetryWorkflowRun)
			})
		})
	})

	r.Route("/sdk/v1", func(r chi.Router) {
		r.Use(s.runTokenAuth)
		r.Route("/runs/{runID}", func(r chi.Router) {
			r.Post("/log", s.handleSDKLog)
			r.Post("/progress", s.handleSDKProgress)
			r.Post("/annotate", s.handleSDKAnnotate)
			r.Post("/heartbeat", s.handleSDKHeartbeat)
			r.Post("/checkpoint", s.handleSDKCheckpoint)
			r.Post("/usage", s.handleSDKUsage)
			r.Post("/tool-call", s.handleSDKToolCall)
			r.Post("/output", s.handleSDKOutput)
			r.Post("/complete", s.handleSDKComplete)
			r.Post("/fail", s.handleSDKFail)
			r.Post("/spawn", s.handleSDKSpawn)
			r.Post("/continue", s.handleSDKContinue)
		})
	})

	// API Reference
	r.Get("/reference", s.handleAPIReference)
	r.Get("/reference/openapi.yaml", s.handleOpenAPISpec)

	return r
}
