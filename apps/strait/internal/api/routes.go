package api

import (
	"log/slog"
	"net/http"
	"time"

	"strait/internal/debug"
	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	sentryhttp "github.com/getsentry/sentry-go/http"
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
	r.Use(securityHeaders)

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(otelchi.Middleware("strait", otelchi.WithChiRoutes(r)))
	r.Use(s.requestLogger)
	r.Use(s.requestMetrics)
	sentryHandler := sentryhttp.New(sentryhttp.Options{
		Repanic:         true,
		WaitForDelivery: false,
	})
	r.Use(sentryHandler.Handle)
	r.Use(chimw.Recoverer)
	r.Use(apiVersionHeader)
	if s.poolStatter != nil {
		r.Use(s.dbBackpressure)
	}
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

	// rateLimit returns a rate limiting middleware when rate limiting is enabled,
	// otherwise a no-op. Rate limiting is considered enabled when either the
	// global limiter (RateLimitRequests) or the trigger-specific limiter
	// (TriggerRateLimitRequests) is configured, so that per-route limits always
	// apply in production but can be disabled entirely in tests by zeroing both.
	rateLimitEnabled := s.config.RateLimitRequests > 0 || s.config.TriggerRateLimitRequests > 0
	rateLimit := func(requests int, window time.Duration) func(http.Handler) http.Handler {
		if !rateLimitEnabled {
			return func(next http.Handler) http.Handler { return next }
		}
		return httprate.LimitByIP(requests, window)
	}

	// Initialize Huma API for auto-generated OpenAPI documentation.
	// Huma wraps the chi router and generates OpenAPI from typed handlers.
	humaConfig := huma.DefaultConfig("Strait API", "1.0.0")
	humaConfig.Info.Description = "Production-grade job orchestration platform for background jobs, workflows, and managed execution."
	humaConfig.Servers = []*huma.Server{
		{URL: "https://strait.fly.dev", Description: "Production"},
	}
	humaConfig.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			Description:  "API key passed as Bearer token",
			BearerFormat: "strait_...",
		},
		"internalSecret": {
			Type:        "apiKey",
			In:          "header",
			Name:        "X-Internal-Secret",
			Description: "Internal service-to-service authentication",
		},
	}
	humaConfig.DocsPath = ""
	humaConfig.OpenAPIPath = ""
	// Separate Huma router for OpenAPI generation only.
	humaRouter := chi.NewRouter()
	s.humaAPI = humachi.New(humaRouter, humaConfig)
	s.registerHumaOperations(s.humaAPI)
	s.cachedOpenAPISpec, _ = s.humaAPI.OpenAPI().MarshalJSON()

	r.Get("/health", s.handleHealth)
	r.Get("/health/ready", s.handleHealthReady)
	if s.metricsHandler != nil {
		r.Handle("/metrics", s.metricsHandler)
	}

	if s.config.DebugStatsviz {
		slog.Warn("statsviz debug endpoints enabled at /debug/statsviz/ -- disable in production")
		debug.MountDebugRoutes(r)
	}

	// Polar billing webhook (HMAC-verified, no API key auth).
	if s.polarWebhook != nil {
		r.Post("/api/webhooks/polar", s.polarWebhook.ServeHTTP)
	}

	// CLI device authorization endpoints (no auth required).
	r.Route("/v1/cli/auth", func(r chi.Router) {
		r.Use(rateLimit(10, time.Minute))
		r.Post("/device-code", s.handleDeviceCode)
		r.Post("/token", s.handleDeviceToken)
	})

	// SSE stream route with query-param token auth for browser EventSource clients.
	// Placed before the main /v1 group so sseTokenAuth runs before apiKeyOrSecretAuth.
	r.Route("/v1/events/{eventKey}/stream", func(r chi.Router) {
		r.Use(s.sseTokenAuth)
		r.Use(s.apiKeyOrSecretAuth)
		r.Use(chimw.Timeout(requestTimeout))
		r.Get("/", s.handleEventTriggerStream)
	})

	// Run stream route without timeout middleware so SSE connections stay open.
	r.Route("/v1/runs/{runID}/stream", func(r chi.Router) {
		r.Use(s.apiKeyOrSecretAuth)
		r.Use(s.projectContextMiddleware)
		r.Use(s.projectRateLimit)
		r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", s.handleRunStream)
	})

	// Org-scoped cross-project query routes.
	r.Route("/v1/organizations/{orgID}", func(r chi.Router) {
		r.Use(s.apiKeyOrSecretAuth)
		r.Use(chimw.Timeout(requestTimeout))
		r.Get("/runs", s.handleListOrgRuns)
		r.Get("/jobs", s.handleListOrgJobs)
	})

	r.Route("/v1", func(r chi.Router) {
		r.Use(s.apiKeyOrSecretAuth)
		r.Use(s.projectContextMiddleware)
		r.Use(s.projectRateLimit)
		r.Use(chimw.Timeout(requestTimeout))
		r.Route("/secrets", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeSecretsWrite), rateLimit(20, time.Minute)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateSecret))
			r.With(s.requirePermission(domain.ScopeSecretsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListSecrets))
			r.With(s.requirePermission(domain.ScopeSecretsWrite)).Delete("/{secretID}", TypedHandler(s, http.StatusNoContent, s.handleDeleteSecret))
		})

		r.Get("/plans", TypedHandler(s, http.StatusOK, s.handleGetPlans))
		r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/regions", s.handleListRegions)

		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/current", s.handleGetCurrentUsage)
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/history", s.handleGetUsageHistory)
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/forecast", s.handleGetUsageForecast)
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/projects", s.handleGetProjectCosts)
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/anomalies", s.handleGetAnomalyAlerts)
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/export", s.handleExportUsage)
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/spending-limit", s.handleGetSpendingLimit)
		r.With(s.requirePermission(domain.ScopeProjectsManage)).Put("/spending-limit", s.handleUpdateSpendingLimit)
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/cost-estimate", s.handleGetCostEstimate)
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/downgrade-preview", s.handleGetDowngradePreview)
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/project-budget", s.handleGetProjectBudget)
		r.With(s.requirePermission(domain.ScopeProjectsManage)).Put("/project-budget", s.handleUpdateProjectBudget)
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/anomaly-config", s.handleGetAnomalyConfig)
		r.With(s.requirePermission(domain.ScopeProjectsManage)).Put("/anomaly-config", s.handleUpdateAnomalyConfig)
		r.Get("/billing/check-org-limit", s.handleCheckOrgLimit)
		r.Route("/referrals", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeProjectsManage)).Post("/", s.handleCreateReferralCode)
			r.With(s.requirePermission(domain.ScopeProjectsManage), rateLimit(5, time.Minute)).Post("/activate", s.handleActivateReferral)
			r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/", s.handleListReferrals)
		})

		r.Route("/projects", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeProjectsManage)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateProject))
			r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListProjects))

			r.Route("/{projectID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetProject))
				r.With(s.requirePermission(domain.ScopeProjectsManage)).Delete("/", TypedHandler(s, http.StatusNoContent, s.handleDeleteProject))

				r.Route("/settings", func(r chi.Router) {
					r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", s.handleGetProjectSettings)
					r.With(s.requirePermission(domain.ScopeJobsWrite)).Put("/", s.handleUpdateProjectSettings)
				})
			})
		})

		r.Route("/jobs", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeJobsWrite), rateLimit(30, time.Minute)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateJob))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListJobs))
			r.With(s.requirePermission(domain.ScopeJobsWrite), rateLimit(10, time.Minute)).Post("/batch", s.handleBatchCreateJobs)
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/batch-enable", TypedHandler(s, http.StatusOK, s.handleBatchEnableJobs))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/batch-disable", TypedHandler(s, http.StatusOK, s.handleBatchDisableJobs))

			r.Route("/{jobID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetJob))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Patch("/", TypedHandler(s, http.StatusOK, s.handleUpdateJob))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", TypedHandler(s, http.StatusNoContent, s.handleDeleteJob))
				r.With(s.requirePermission(domain.ScopeJobsTrigger), rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/trigger", s.handleTriggerJob)
				r.With(s.requirePermission(domain.ScopeJobsTrigger), rateLimit(5, time.Minute)).Post("/trigger/bulk", s.handleBulkTriggerJob)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/dependencies", s.handleCreateJobDependency)
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/dependencies", s.handleListJobDependencies)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/dependencies/{depID}", s.handleDeleteJobDependency)
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/versions", s.handleListJobVersions)
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/versions/{versionID}", s.handleGetJobVersion)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/clone", TypedHandler(s, http.StatusCreated, s.handleCloneJob))
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/health", TypedHandler(s, http.StatusOK, s.handleGetJobHealth))
			})
		})

		r.Route("/job-groups", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/", s.handleCreateJobGroup)
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", s.handleListJobGroups)
			r.Route("/{groupID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", s.handleGetJobGroup)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Patch("/", s.handleUpdateJobGroup)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", s.handleDeleteJobGroup)
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/jobs", s.handleListJobsByGroup)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/pause-all", s.handlePauseAllJobsByGroup)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/resume-all", s.handleResumeAllJobsByGroup)
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/stats", s.handleGetJobGroupStats)
			})
		})

		r.Route("/environments", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateEnvironment))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListEnvironments))
			r.Route("/{envID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetEnvironment))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Patch("/", TypedHandler(s, http.StatusOK, s.handleUpdateEnvironment))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", TypedHandler(s, http.StatusNoContent, s.handleDeleteEnvironment))
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/variables", TypedHandler(s, http.StatusOK, s.handleGetResolvedVariables))
			})
		})

		r.Route("/runs", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", s.handleListRuns)
			r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/dlq", s.handleListDeadLetterRuns)
			r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/bulk-dlq-replay", s.handleBulkReplayDeadLetterRuns)
			r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/bulk-cancel", s.handleBulkCancelRuns)
			r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/bulk-cancel-all", s.handleBulkCancelAll)
			r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/bulk-replay", s.handleBulkReplayRuns)
			r.Route("/{runID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", s.handleGetRun)
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Delete("/", s.handleCancelRun)
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/replay", s.handleReplayRun)
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/dlq-replay", s.handleReplayDeadLetterRun)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/children", s.handleListChildRuns)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/events", s.handleListRunEvents)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/checkpoints", s.handleListRunCheckpoints)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/usage", s.handleListRunUsage)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/tool-calls", s.handleListRunToolCalls)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/outputs", s.handleListRunOutputs)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/debug-bundle", s.handleGetDebugBundle)
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/debug", s.handleSetDebugMode)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/lineage", s.handleListRunLineage)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/dependency-status", s.handleGetRunDependencyStatus)
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Delete("/idempotency-key", s.handleResetIdempotencyKey)
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/reschedule", s.handleRescheduleRun)
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/pause", s.handlePauseRun)
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/resume", s.handleResumeRun)
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/restart", s.handleRestartRun)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/state", s.handleListRunState)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/stream/chunks", s.handleRunLLMStream)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/resources", s.handleListRunResources)
			})
		})

		r.Route("/batch-operations", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", s.handleListBatchOperations)
			r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/{batchID}", s.handleGetBatchOperation)
		})

		r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/webhook-deliveries", s.handleListWebhookDeliveries)
		r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/webhook-deliveries/{deliveryID}/retry", s.handleRetryWebhookDelivery)

		r.Route("/webhooks", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeRunsWrite), rateLimit(5, time.Minute)).Post("/test", s.handleTestWebhook)
			r.Route("/deliveries", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", s.handleListWebhookDeliveries)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/{id}", s.handleGetWebhookDelivery)
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/{id}/retry", s.handleRetryWebhookDelivery)
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/{id}/replay", s.handleReplayWebhookDelivery)
			})
			r.Route("/subscriptions", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/", s.handleCreateWebhookSubscription)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", s.handleListWebhookSubscriptions)
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Delete("/{id}", s.handleDeleteWebhookSubscription)
			})
		})

		r.Route("/notification-channels", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/", s.handleCreateNotificationChannel)
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", s.handleListNotificationChannels)
			r.Route("/{channelID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", s.handleGetNotificationChannel)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Patch("/", s.handleUpdateNotificationChannel)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", s.handleDeleteNotificationChannel)
			})
		})
		r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/notification-deliveries", s.handleListNotificationDeliveries)

		r.Route("/log-drains", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", s.handleListLogDrains)
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/", s.handleCreateLogDrain)
			r.Route("/{drainID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", s.handleGetLogDrain)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Patch("/", s.handleUpdateLogDrain)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", s.handleDeleteLogDrain)
			})
		})

		r.Route("/api-keys", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeAPIKeysManage), httprate.LimitByIP(10, time.Minute)).Post("/", s.handleCreateAPIKey)
			r.With(s.requirePermission(domain.ScopeAPIKeysManage)).Get("/", s.handleListAPIKeys)
			r.With(s.requirePermission(domain.ScopeAPIKeysManage), rateLimit(10, time.Minute)).Post("/{keyID}/rotate", s.handleRotateAPIKey)
			r.With(s.requirePermission(domain.ScopeAPIKeysManage)).Delete("/{keyID}", s.handleRevokeAPIKey)
		})

		r.With(s.requirePermission(domain.ScopeAPIKeysManage)).Post("/cli/device-codes/approve", s.handleApproveDeviceCode)

		r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/stats", s.handleStats)

		r.Route("/analytics", func(r chi.Router) {
			// Community analytics (Postgres-backed, always available)
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/performance", s.handleGetPerformanceAnalytics)
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/costs", s.handleGetCostAnalytics)
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/costs/trends", s.handleGetCostTrends)
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/costs/top", s.handleGetTopCosts)
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/compute", s.handleGetComputeCostAnalytics)
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/approvals", s.handleGetApprovalStats)
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/cost-insights", s.handleGetCostInsights)

			// Cloud-only analytics (ClickHouse-backed, requires Strait Cloud)
			r.Group(func(r chi.Router) {
				r.Use(s.requireCloudEdition)
				r.Use(s.requirePermission(domain.ScopeStatsRead))

				r.Route("/runs", func(r chi.Router) {
					r.Get("/timeline", s.handleRunTimeline)
					r.Get("/duration-distribution", s.handleRunDurationDistribution)
					r.Get("/failure-reasons", s.handleRunFailureReasons)
					r.Get("/summary", s.handleRunSummary)
					r.Get("/by-trigger", s.handleRunsByTrigger)
				})

				r.Route("/jobs", func(r chi.Router) {
					r.Get("/comparison", s.handleJobComparison)
					r.Get("/reliability", s.handleJobReliability)
					r.Get("/by-version", s.handleRunsByVersion)
					r.Get("/cost-ranking", s.handleJobCostRanking)
					r.Get("/top-failing", s.handleTopFailingJobs)
					r.Get("/{jobID}/history", s.handleJobHistory)
				})

				r.Route("/tags", func(r chi.Router) {
					r.Get("/summary", s.handleTagSummary)
					r.Get("/top-failing", s.handleTopFailingTags)
					r.Get("/cost", s.handleTagCost)
				})

				r.Route("/workflows", func(r chi.Router) {
					r.Get("/completion-rates", s.handleWorkflowCompletionRates)
					r.Get("/summary", s.handleWorkflowAnalyticsSummary)
					r.Get("/{workflowID}/step-durations", s.handleWorkflowStepDurations)
				})

				r.Route("/webhooks", func(r chi.Router) {
					r.Get("/delivery-stats", s.handleWebhookDeliveryStats)
					r.Get("/endpoint-health", s.handleWebhookEndpointHealth)
					r.Get("/top-failing", s.handleTopFailingWebhooks)
				})

				r.Route("/events", func(r chi.Router) {
					r.Get("/volume", s.handleEventVolume)
					r.Get("/latency", s.handleEventLatency)
				})

				r.Get("/costs/forecast", s.handleCostForecast)
				r.Get("/costs/by-trigger", s.handleCostByTrigger)
				r.Get("/costs/by-machine", s.handleCostByMachine)
			})
		})

		r.Route("/roles", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeRBACManage), rateLimit(20, time.Minute)).Post("/", s.handleCreateRole)
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/", s.handleListRoles)
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/{roleID}", s.handleGetRole)
			r.With(s.requirePermission(domain.ScopeRBACManage), rateLimit(20, time.Minute)).Patch("/{roleID}", s.handleUpdateRole)
			r.With(s.requirePermission(domain.ScopeRBACManage), rateLimit(20, time.Minute)).Delete("/{roleID}", s.handleDeleteRole)
		})

		r.Route("/members", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeRBACManage), rateLimit(40, time.Minute)).Post("/", s.handleAssignMember)
			r.With(s.requirePermission(domain.ScopeRBACManage), rateLimit(20, time.Minute)).Post("/bulk", s.handleBulkAssignMembers)
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/", s.handleListMembers)
			r.With(s.requirePermission(domain.ScopeRBACManage), rateLimit(40, time.Minute)).Delete("/{userID}", s.handleRemoveMember)
		})

		r.With(s.requirePermission(domain.ScopeRBACManage), rateLimit(5, time.Minute)).Post("/seed-roles", s.handleSeedSystemRoles)
		r.Route("/audit-events", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/", s.handleListAuditEvents)
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/export", s.handleExportAuditEvents)
		})

		r.Route("/resource-policies", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeRBACManage)).Post("/", s.handleCreateResourcePolicy)
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/", s.handleListResourcePolicies)
			r.With(s.requirePermission(domain.ScopeRBACManage)).Delete("/{policyID}", s.handleDeleteResourcePolicy)
		})

		r.Route("/tag-policies", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeRBACManage)).Post("/", s.handleCreateTagPolicy)
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/", s.handleListTagPolicies)
			r.With(s.requirePermission(domain.ScopeRBACManage)).Delete("/{policyID}", s.handleDeleteTagPolicy)
		})

		r.Route("/workflow-policies", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/{projectID}", s.handleGetWorkflowPolicy)
			r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Put("/{projectID}", s.handleUpsertWorkflowPolicy)
		})

		r.Route("/workflows", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/", s.handleCreateWorkflow)
			r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/", s.handleListWorkflows)
			r.Route("/{workflowID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/", s.handleGetWorkflow)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Patch("/", s.handleUpdateWorkflow)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Delete("/", s.handleDeleteWorkflow)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Post("/dry-run", s.handleDryRunWorkflow)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Post("/plan", s.handleWorkflowPlan)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Post("/simulate", s.handleSimulateWorkflow)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/graph", s.handleWorkflowGraph)
				r.With(s.requirePermission(domain.ScopeWorkflowsTrigger)).Post("/trigger", s.handleTriggerWorkflow)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/clone", s.handleCloneWorkflow)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/runs", s.handleListWorkflowRuns)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/versions", s.handleListWorkflowVersions)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/versions/{versionID}", s.handleGetWorkflowVersion)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/versions/{versionID}/steps", s.handleListWorkflowVersionSteps)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/versions/{fromVersionID}/diff/{toVersionID}", s.handleWorkflowVersionDiff)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/versions/{versionID}/impact", s.handleWorkflowVersionImpact)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/active-versions", s.handleGetActiveVersions)
			})
		})

		r.Route("/deployments", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/", s.handleCreateDeploymentVersion)
			r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/", s.handleListDeploymentVersions)
			r.Route("/{deploymentID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/finalize", s.handleFinalizeDeploymentVersion)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/promote", s.handlePromoteDeploymentVersion)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/rollback", s.handleRollbackDeploymentVersion)
			})
		})

		r.Route("/event-sources", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", s.handleListEventSources)
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/", s.handleCreateEventSource)
			r.Route("/{sourceID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", s.handleGetEventSource)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Patch("/", s.handleUpdateEventSource)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", s.handleDeleteEventSource)
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/subscriptions", s.handleListEventSourceSubscriptions)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/subscribe", s.handleSubscribeToEventSource)
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/subscriptions/{subID}", s.handleDeleteEventSubscription)
			})
		})
		r.With(
			s.requirePermission(domain.ScopeJobsWrite),
			rateLimit(triggerRateLimitRequests, triggerRateLimitWindow),
		).Post("/events/dispatch", s.handleDispatchEvent)

		r.Route("/events", func(r chi.Router) {
			r.Get("/", s.handleListEventTriggers)
			r.Get("/stats", s.handleGetEventTriggerStats)
			r.Post("/purge", s.handlePurgeEventTriggers)
			r.Route("/prefix/{prefix}", func(r chi.Router) {
				r.With(rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/send", s.handleSendEventByPrefix)
			})
			r.Route("/{eventKey}", func(r chi.Router) {
				r.Get("/", s.handleGetEventTrigger)
				r.Delete("/", s.handleCancelEventTrigger)
				r.With(rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/send", s.handleSendEvent)
			})
		})

		r.Route("/workflow-runs", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/", s.handleListWorkflowRunsByProject)
			r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/bulk-cancel", s.handleBulkCancelWorkflowRuns)
			r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/bulk-replay", s.handleBulkReplayWorkflowRuns)
			r.Route("/{workflowRunID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/", s.handleGetWorkflowRun)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Delete("/", s.handleCancelWorkflowRun)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/pause", s.handlePauseWorkflowRun)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/resume", s.handleResumeWorkflowRun)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/labels", s.handleGetWorkflowRunLabels)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/steps", s.handleListWorkflowStepRuns)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/graph", s.handleGetWorkflowRunGraph)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/explain", s.handleGetWorkflowRunExplain)
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/timeline", s.handleGetWorkflowRunTimeline)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/approve", s.handleApproveWorkflowStep)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/skip", s.handleSkipWorkflowStep)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/force-complete", s.handleForceCompleteWorkflowStep)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/retry", s.handleRetryWorkflowStep)
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/replay-subtree", s.handleReplayWorkflowSubtree)
				r.With(s.requirePermission(domain.ScopeWorkflowsTrigger)).Post("/retry", s.handleRetryWorkflowRun)
			})
		})
	})

	r.Route("/sdk/v1", func(r chi.Router) {
		r.Use(s.runTokenAuth)
		r.Route("/runs/{runID}", func(r chi.Router) {
			r.Get("/payload", s.handleSDKGetPayload)
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
			r.Post("/state", s.handleSDKSetState)
			r.Get("/state", s.handleSDKListState)
			r.Get("/state/{key}", s.handleSDKGetState)
			r.Delete("/state/{key}", s.handleSDKDeleteState)
			r.Post("/stream", s.handleSDKStreamChunk)
			r.Post("/resources", s.handleSDKResources)
			r.Post("/resource-snapshot", s.handleSDKResourceSnapshot)
			r.Post("/iteration", s.handleSDKIteration)
			r.Route("/memory", func(r chi.Router) {
				r.Post("/{key}", s.handleSDKSetMemory)
				r.Get("/{key}", s.handleSDKGetMemory)
				r.Get("/", s.handleSDKListMemory)
				r.Delete("/{key}", s.handleSDKDeleteMemory)
			})
		})
	})

	// API Reference
	r.Get("/reference", s.handleAPIReference)
	r.Get("/reference/openapi.yaml", s.handleOpenAPISpec)

	return r
}
