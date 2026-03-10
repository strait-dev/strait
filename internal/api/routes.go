package api

import (
	"net/http"
	"time"

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
	r.Use(otelchi.Middleware("strait", otelchi.WithChiRoutes(r)))
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
			r.With(rateLimit(20, time.Minute)).Post("/", s.handleCreateSecret)
			r.Get("/", s.handleListSecrets)
			r.Delete("/{secretID}", s.handleDeleteSecret)
		})

		r.Route("/jobs", func(r chi.Router) {
			r.With(rateLimit(30, time.Minute)).Post("/", s.handleCreateJob)
			r.Get("/", s.handleListJobs)
			r.With(rateLimit(10, time.Minute)).Post("/batch", s.handleBatchCreateJobs)
			r.Post("/batch-enable", s.handleBatchEnableJobs)
			r.Post("/batch-disable", s.handleBatchDisableJobs)

			r.Route("/{jobID}", func(r chi.Router) {
				r.Get("/", s.handleGetJob)
				r.Patch("/", s.handleUpdateJob)
				r.Delete("/", s.handleDeleteJob)
				r.With(rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/trigger", s.handleTriggerJob)
				r.With(rateLimit(5, time.Minute)).Post("/trigger/bulk", s.handleBulkTriggerJob)
				r.Post("/dependencies", s.handleCreateJobDependency)
				r.Get("/dependencies", s.handleListJobDependencies)
				r.Delete("/dependencies/{depID}", s.handleDeleteJobDependency)
				r.Get("/versions", s.handleListJobVersions)
				r.Post("/clone", s.handleCloneJob)
				r.Get("/health", s.handleGetJobHealth)
			})
		})

		r.Route("/job-groups", func(r chi.Router) {
			r.Post("/", s.handleCreateJobGroup)
			r.Get("/", s.handleListJobGroups)
			r.Route("/{groupID}", func(r chi.Router) {
				r.Get("/", s.handleGetJobGroup)
				r.Patch("/", s.handleUpdateJobGroup)
				r.Delete("/", s.handleDeleteJobGroup)
				r.Get("/jobs", s.handleListJobsByGroup)
			})
		})

		r.Route("/environments", func(r chi.Router) {
			r.Post("/", s.handleCreateEnvironment)
			r.Get("/", s.handleListEnvironments)
			r.Route("/{envID}", func(r chi.Router) {
				r.Get("/", s.handleGetEnvironment)
				r.Patch("/", s.handleUpdateEnvironment)
				r.Delete("/", s.handleDeleteEnvironment)
				r.Get("/variables", s.handleGetResolvedVariables)
			})
		})

		r.Route("/runs", func(r chi.Router) {
			r.Get("/", s.handleListRuns)
			r.Get("/dlq", s.handleListDeadLetterRuns)
			r.Post("/bulk-cancel", s.handleBulkCancelRuns)
			r.Route("/{runID}", func(r chi.Router) {
				r.Get("/", s.handleGetRun)
				r.Delete("/", s.handleCancelRun)
				r.Post("/replay", s.handleReplayRun)
				r.Post("/dlq-replay", s.handleReplayDeadLetterRun)
				r.Get("/stream", s.handleRunStream)
				r.Get("/children", s.handleListChildRuns)
				r.Get("/events", s.handleListRunEvents)
				r.Get("/checkpoints", s.handleListRunCheckpoints)
				r.Get("/usage", s.handleListRunUsage)
				r.Get("/tool-calls", s.handleListRunToolCalls)
				r.Get("/outputs", s.handleListRunOutputs)
				r.Get("/debug-bundle", s.handleGetDebugBundle)
				r.Post("/debug", s.handleSetDebugMode)
				r.Get("/lineage", s.handleListRunLineage)
			})
		})

		r.Get("/webhook-deliveries", s.handleListWebhookDeliveries)

		r.Route("/api-keys", func(r chi.Router) {
			r.With(httprate.LimitByIP(10, time.Minute)).Post("/", s.handleCreateAPIKey)
			r.Get("/", s.handleListAPIKeys)
			r.Delete("/{keyID}", s.handleRevokeAPIKey)
		})

		r.Get("/stats", s.handleStats)

		r.Route("/workflows", func(r chi.Router) {
			r.Post("/", s.handleCreateWorkflow)
			r.Get("/", s.handleListWorkflows)
			r.Route("/{workflowID}", func(r chi.Router) {
				r.Get("/", s.handleGetWorkflow)
				r.Patch("/", s.handleUpdateWorkflow)
				r.Delete("/", s.handleDeleteWorkflow)
				r.Post("/dry-run", s.handleDryRunWorkflow)
				r.Get("/graph", s.handleWorkflowGraph)
				r.Post("/trigger", s.handleTriggerWorkflow)
				r.Post("/clone", s.handleCloneWorkflow)
				r.Get("/runs", s.handleListWorkflowRuns)
			})
		})

		r.Route("/events", func(r chi.Router) {
			r.Get("/", s.handleListEventTriggers)
			r.Get("/stats", s.handleGetEventTriggerStats)
			r.Route("/prefix/{prefix}", func(r chi.Router) {
				r.With(rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/send", s.handleSendEventByPrefix)
			})
			r.Route("/{eventKey}", func(r chi.Router) {
				r.Get("/", s.handleGetEventTrigger)
				r.Delete("/", s.handleCancelEventTrigger)
				r.Get("/stream", s.handleEventTriggerStream)
				r.With(rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/send", s.handleSendEvent)
			})
		})

		r.Route("/workflow-runs", func(r chi.Router) {
			r.Get("/", s.handleListWorkflowRunsByProject)
			r.Route("/{workflowRunID}", func(r chi.Router) {
				r.Get("/", s.handleGetWorkflowRun)
				r.Delete("/", s.handleCancelWorkflowRun)
				r.Post("/pause", s.handlePauseWorkflowRun)
				r.Post("/resume", s.handleResumeWorkflowRun)
				r.Get("/labels", s.handleGetWorkflowRunLabels)
				r.Get("/steps", s.handleListWorkflowStepRuns)
				r.Post("/steps/{stepRef}/approve", s.handleApproveWorkflowStep)
				r.Post("/steps/{stepRef}/skip", s.handleSkipWorkflowStep)
				r.Post("/steps/{stepRef}/force-complete", s.handleForceCompleteWorkflowStep)
				r.Post("/retry", s.handleRetryWorkflowRun)
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
			r.Post("/wait-for-event", s.handleSDKWaitForEvent)
		})
	})

	// API Reference
	r.Get("/reference", s.handleAPIReference)
	r.Get("/reference/openapi.yaml", s.handleOpenAPISpec)

	return r
}
