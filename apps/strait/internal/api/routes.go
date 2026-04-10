package api

import (
	"log/slog"
	"net/http"
	"sync"
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

// cachedOpenAPIOnce ensures Huma operation registration and OpenAPI spec
// generation happen exactly once per process. The Huma API and serialized
// spec are identical for every server instance (they depend only on handler
// types and metadata, not on per-request state), so registering 257
// operations on every NewServer call is pure waste -- especially under the
// race detector where ~300 test servers amplify the cost.
var (
	cachedOpenAPIOnce sync.Once
	cachedHumaSpec    []byte
)

func (s *Server) routes() chi.Router {
	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.config.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Secret", "X-Idempotency-Key", "Idempotency-Key"},
		ExposedHeaders:   []string{"Link", "X-Request-Id", "X-API-Version", "X-Strait-Plan", "X-Strait-Usage-Limit", "X-Strait-Usage-Remaining"},
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
	// Registration is expensive (257 operations via reflection) so we do it
	// once per process via sync.Once. The spec is identical for every server
	// since it depends only on handler types, not runtime state.
	cachedOpenAPIOnce.Do(func() {
		humaConfig := huma.DefaultConfig("Strait API", "1.0.0")
		humaConfig.Info.Description = "Production-grade job orchestration platform for background jobs, workflows, and managed execution."
		humaConfig.Servers = []*huma.Server{
			{URL: "https://api.strait.dev", Description: "Production"},
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
		humaRouter := chi.NewRouter()
		api := humachi.New(humaRouter, humaConfig)
		registerAllTypedOps(api, s)
		s.registerHumaOperations(api)
		cachedHumaSpec, _ = api.OpenAPI().MarshalJSON()
	})
	s.cachedOpenAPISpec = cachedHumaSpec

	r.Get("/health", s.handleHealth)
	r.Get("/health/ready", s.handleHealthReady)
	if s.metricsHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(s.internalSecretAuth)
			r.Handle("/metrics", s.metricsHandler)
		})
	}

	if s.config.DebugStatsviz {
		slog.Warn("statsviz debug endpoints enabled at /debug/statsviz/ -- disable in production")
		r.Group(func(r chi.Router) {
			r.Use(s.internalSecretAuth)
			debug.MountDebugRoutes(r)
		})
	}

	// Stripe billing webhook (signature-verified, no API key auth).
	if s.stripeWebhook != nil {
		r.Post("/api/webhooks/stripe", s.stripeWebhook.ServeHTTP)
	}

	// CDC webhook (Sequin push delivery, internal secret auth).
	if s.cdcWebhookReceiver != nil {
		r.Route("/internal/cdc", func(r chi.Router) {
			r.Use(s.internalSecretAuth)
			r.Post("/webhook", s.cdcWebhookReceiver.ServeHTTP)
		})
	}

	// Admin internal routes (X-Internal-Secret auth only).
	r.Route("/internal/admin", func(r chi.Router) {
		r.Use(s.internalSecretAuth)
		r.Use(chimw.Timeout(requestTimeout))
		r.Route("/orgs/{orgID}", func(r chi.Router) {
			r.Get("/deployments", TypedHandler(s, http.StatusOK, s.handleListAdminOrgDeployments))
		})
	})

	// CLI device authorization endpoints (no auth required).
	r.Route("/v1/cli/auth", func(r chi.Router) {
		r.Use(rateLimit(10, time.Minute))
		r.Post("/device-code", TypedHandler(s, http.StatusOK, s.handleDeviceCode))
		r.Post("/token", TypedHandler(s, http.StatusOK, s.handleDeviceToken))
	})

	// SSE stream route with query-param token auth for browser EventSource clients.
	// Placed before the main /v1 group so sseTokenAuth runs before apiKeyOrSecretAuth.
	r.Route("/v1/events/{eventKey}/stream", func(r chi.Router) {
		r.Use(s.sseTokenAuth)
		r.Use(s.apiKeyOrSecretAuth)
		r.Use(chimw.Timeout(requestTimeout))
		r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", s.handleEventTriggerStream)
	})

	// Run stream route without timeout middleware so SSE connections stay open.
	r.Route("/v1/runs/{runID}/stream", func(r chi.Router) {
		r.Use(s.apiKeyOrSecretAuth)
		r.Use(s.projectContextMiddleware)
		r.Use(s.projectRateLimit)
		r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", s.handleRunStream)
	})

	// Project activity stream (SSE, no timeout -- connections stay open).
	r.Route("/v1/projects/{projectID}/activity/stream", func(r chi.Router) {
		r.Use(s.apiKeyOrSecretAuth)
		r.Use(s.projectContextMiddleware)
		r.Use(s.projectRateLimit)
		r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", s.handleProjectActivityStream)
	})

	// Deployment build log stream (SSE when stream=true and build is in progress).
	// Placed before /v1 to avoid the global request timeout middleware.
	r.Route("/v1/jobs/{jobID}/deployments/{deploymentID}/logs", func(r chi.Router) {
		r.Use(s.apiKeyOrSecretAuth)
		r.Use(s.projectContextMiddleware)
		r.Use(s.projectRateLimit)
		r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", s.handleDeploymentLogs)
	})

	// Org-scoped cross-project query routes.
	r.Route("/v1/organizations/{orgID}", func(r chi.Router) {
		r.Use(s.apiKeyOrSecretAuth)
		r.Use(chimw.Timeout(requestTimeout))
		r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/runs", TypedHandler(s, http.StatusOK, s.handleListOrgRuns))
		r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/jobs", TypedHandler(s, http.StatusOK, s.handleListOrgJobs))
	})

	// Subscriber-authenticated inbox and preference endpoints.
	r.Route("/v1/inbox", func(r chi.Router) {
		r.Use(s.notifySubscriberAuth)
		r.Use(s.projectContextMiddleware)
		r.Use(s.projectRateLimit)
		r.Get("/feed", s.handleInboxFeed)
		r.Get("/", TypedHandler(s, http.StatusOK, s.handleListInbox))
		r.Get("/unread-count", TypedHandler(s, http.StatusOK, s.handleInboxUnreadCount))
		r.Patch("/{itemID}", TypedHandler(s, http.StatusOK, s.handleUpdateInboxItem))
		r.Post("/{itemID}/action", TypedHandler(s, http.StatusOK, s.handleInboxAction))
		r.Post("/mark-all-read", TypedHandler(s, http.StatusOK, s.handleInboxMarkAllRead))
	})

	r.Route("/v1/preferences", func(r chi.Router) {
		r.Use(s.notifySubscriberAuth)
		r.Use(s.projectContextMiddleware)
		r.Use(s.projectRateLimit)
		r.Get("/", TypedHandler(s, http.StatusOK, s.handleGetNotifyPreferences))
		r.Put("/", TypedHandler(s, http.StatusOK, s.handleUpdateNotifyPreferences))
		r.Put("/{scope}", TypedHandler(s, http.StatusOK, s.handleUpdateNotifyPreferencesScope))
	})

	r.Get("/v1/unsubscribe/{token}", TypedHandler(s, http.StatusOK, s.handleGetUnsubscribe))
	r.Post("/v1/unsubscribe/{token}", TypedHandler(s, http.StatusOK, s.handleProcessUnsubscribe))
	r.Post("/v1/unsubscribe/{token}/one-click", TypedHandler(s, http.StatusOK, s.handleUnsubscribeOneClick))

	// Email tracking endpoints are public by design.
	r.Get("/t/{messageID}/open.gif", s.handleNotifyOpenPixel)
	r.Get("/t/{messageID}/click", s.handleNotifyClickRedirect)

	r.Route("/v1", func(r chi.Router) {
		r.Use(s.apiKeyOrSecretAuth)
		r.Use(requireJSONAccept)
		r.Use(requireJSONContentType)
		r.Use(s.projectContextMiddleware)
		r.Use(s.projectRateLimit)
		r.Use(s.planUsageHeaders)
		r.Use(chimw.Timeout(requestTimeout))
		r.Route("/secrets", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeSecretsWrite), rateLimit(20, time.Minute)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateSecret))
			r.With(s.requirePermission(domain.ScopeSecretsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListSecrets))
			r.With(s.requirePermission(domain.ScopeSecretsWrite)).Delete("/{secretID}", TypedHandler(s, http.StatusNoContent, s.handleDeleteSecret))
		})

		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/plans", TypedHandler(s, http.StatusOK, s.handleGetPlans))
		r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/regions", TypedHandler(s, http.StatusOK, s.handleListRegions))

		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/current", TypedHandler(s, http.StatusOK, s.handleGetCurrentUsage))
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/history", TypedHandler(s, http.StatusOK, s.handleGetUsageHistory))
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/forecast", TypedHandler(s, http.StatusOK, s.handleGetUsageForecast))
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/projects", TypedHandler(s, http.StatusOK, s.handleGetProjectCosts))
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/anomalies", TypedHandler(s, http.StatusOK, s.handleGetAnomalyAlerts))
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/export", TypedHandler(s, http.StatusOK, s.handleExportUsage))
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/spending-limit", TypedHandler(s, http.StatusOK, s.handleGetSpendingLimit))
		r.With(s.requirePermission(domain.ScopeProjectsManage)).Put("/spending-limit", TypedHandler(s, http.StatusOK, s.handleUpdateSpendingLimit))
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/cost-estimate", TypedHandler(s, http.StatusOK, s.handleGetCostEstimate))
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/downgrade-preview", TypedHandler(s, http.StatusOK, s.handleGetDowngradePreview))
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/project-budget", TypedHandler(s, http.StatusOK, s.handleGetProjectBudget))
		r.With(s.requirePermission(domain.ScopeProjectsManage)).Put("/project-budget", TypedHandler(s, http.StatusOK, s.handleUpdateProjectBudget))
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/anomaly-config", TypedHandler(s, http.StatusOK, s.handleGetAnomalyConfig))
		r.With(s.requirePermission(domain.ScopeProjectsManage)).Put("/anomaly-config", TypedHandler(s, http.StatusOK, s.handleUpdateAnomalyConfig))
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/usage/email-preferences", TypedHandler(s, http.StatusOK, s.handleGetEmailPreferences))
		r.With(s.requirePermission(domain.ScopeProjectsManage)).Put("/usage/email-preferences", TypedHandler(s, http.StatusOK, s.handleUpdateEmailPreferences))
		r.With(s.requirePermission(domain.ScopeProjectsManage)).Post("/cost-estimate/what-if", TypedHandler(s, http.StatusOK, s.handleWhatIfCostEstimate))
		r.With(s.requirePermission(domain.ScopeProjectsManage)).Post("/cost-estimate/deployment", TypedHandler(s, http.StatusOK, s.handleEstimateDeploymentDelta))
		r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/billing/check-org-limit", TypedHandler(s, http.StatusOK, s.handleCheckOrgLimit))
		r.Route("/projects", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeProjectsManage)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateProject))
			r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListProjects))

			r.Route("/{projectID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeProjectsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetProject))
				r.With(s.requirePermission(domain.ScopeProjectsManage)).Delete("/", TypedHandler(s, http.StatusNoContent, s.handleDeleteProject))

				r.Route("/settings", func(r chi.Router) {
					r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetProjectSettings))
					r.With(s.requirePermission(domain.ScopeJobsWrite)).Put("/", TypedHandler(s, http.StatusOK, s.handleUpdateProjectSettings))
				})
			})
		})

		r.Route("/jobs", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite), rateLimit(30, time.Minute)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateJob))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListJobs))
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite), rateLimit(10, time.Minute)).Post("/batch", TypedHandler(s, http.StatusCreated, s.handleBatchCreateJobs))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/batch-enable", TypedHandler(s, http.StatusOK, s.handleBatchEnableJobs))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/batch-disable", TypedHandler(s, http.StatusOK, s.handleBatchDisableJobs))

			r.Route("/{jobID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetJob))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Patch("/", TypedHandler(s, http.StatusOK, s.handleUpdateJob))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", TypedHandler(s, http.StatusNoContent, s.handleDeleteJob))
				r.With(s.requirePermission(domain.ScopeJobsTrigger), rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/trigger", TypedHandler(s, http.StatusCreated, s.handleTriggerJob))
				r.With(s.requirePermission(domain.ScopeJobsTrigger), rateLimit(5, time.Minute)).Post("/trigger/bulk", TypedHandler(s, http.StatusCreated, s.handleBulkTriggerJob))
				r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/dependencies", TypedHandler(s, http.StatusCreated, s.handleCreateJobDependency))
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/dependencies", TypedHandler(s, http.StatusOK, s.handleListJobDependencies))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/dependencies/{depID}", TypedHandler(s, http.StatusNoContent, s.handleDeleteJobDependency))
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/versions", TypedHandler(s, http.StatusOK, s.handleListJobVersions))
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/versions/{versionID}", TypedHandler(s, http.StatusOK, s.handleGetJobVersion))
				r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/clone", TypedHandler(s, http.StatusCreated, s.handleCloneJob))
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/health", TypedHandler(s, http.StatusOK, s.handleGetJobHealth))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/pause", TypedHandler(s, http.StatusOK, s.handlePauseJob))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/resume", TypedHandler(s, http.StatusOK, s.handleResumeJob))

				// Code-first deployment routes.
				r.Route("/deployments", func(r chi.Router) {
					r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusOK, s.handleCreateCodeDeployment))
					r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListCodeDeployments))
					r.Route("/{deploymentID}", func(r chi.Router) {
						r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetCodeDeployment))
						r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/confirm", TypedHandler(s, http.StatusOK, s.handleConfirmCodeDeployment))
						r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/rollback", TypedHandler(s, http.StatusOK, s.handleRollbackCodeDeployment))
					})
				})
			})
		})

		r.Route("/job-groups", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateJobGroup))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListJobGroups))
			r.Route("/{groupID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetJobGroup))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Patch("/", TypedHandler(s, http.StatusOK, s.handleUpdateJobGroup))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", TypedHandler(s, http.StatusNoContent, s.handleDeleteJobGroup))
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/jobs", TypedHandler(s, http.StatusOK, s.handleListJobsByGroup))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/pause-all", TypedHandler(s, http.StatusOK, s.handlePauseAllJobsByGroup))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/resume-all", TypedHandler(s, http.StatusOK, s.handleResumeAllJobsByGroup))
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/stats", TypedHandler(s, http.StatusOK, s.handleGetJobGroupStats))
			})
		})

		r.Route("/environments", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateEnvironment))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListEnvironments))
			r.Route("/{envID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetEnvironment))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Patch("/", TypedHandler(s, http.StatusOK, s.handleUpdateEnvironment))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", TypedHandler(s, http.StatusNoContent, s.handleDeleteEnvironment))
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/variables", TypedHandler(s, http.StatusOK, s.handleGetResolvedVariables))
			})
		})

		r.Route("/runs", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListRuns))
			r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/dlq", TypedHandler(s, http.StatusOK, s.handleListDeadLetterRuns))
			r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/bulk-dlq-replay", TypedHandler(s, http.StatusOK, s.handleBulkReplayDeadLetterRuns))
			r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/bulk-cancel", TypedHandler(s, http.StatusOK, s.handleBulkCancelRuns))
			r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/bulk-cancel-all", TypedHandler(s, http.StatusOK, s.handleBulkCancelAll))
			r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/bulk-replay", TypedHandler(s, http.StatusOK, s.handleBulkReplayRuns))
			r.Route("/{runID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetRun))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Delete("/", TypedHandler(s, http.StatusOK, s.handleCancelRun))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/replay", TypedHandler(s, http.StatusCreated, s.handleReplayRun))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/dlq-replay", TypedHandler(s, http.StatusOK, s.handleReplayDeadLetterRun))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/children", TypedHandler(s, http.StatusOK, s.handleListChildRuns))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/events", TypedHandler(s, http.StatusOK, s.handleListRunEvents))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/checkpoints", TypedHandler(s, http.StatusOK, s.handleListRunCheckpoints))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/usage", TypedHandler(s, http.StatusOK, s.handleListRunUsage))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/tool-calls", TypedHandler(s, http.StatusOK, s.handleListRunToolCalls))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/outputs", TypedHandler(s, http.StatusOK, s.handleListRunOutputs))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/debug-bundle", TypedHandler(s, http.StatusOK, s.handleGetDebugBundle))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/debug", TypedHandler(s, http.StatusOK, s.handleSetDebugMode))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/lineage", TypedHandler(s, http.StatusOK, s.handleListRunLineage))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/dependency-status", TypedHandler(s, http.StatusOK, s.handleGetRunDependencyStatus))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Delete("/idempotency-key", TypedHandler(s, http.StatusOK, s.handleResetIdempotencyKey))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/reschedule", TypedHandler(s, http.StatusOK, s.handleRescheduleRun))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/pause", TypedHandler(s, http.StatusOK, s.handlePauseRun))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/resume", TypedHandler(s, http.StatusOK, s.handleResumeRun))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/restart", TypedHandler(s, http.StatusOK, s.handleRestartRun))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/state", TypedHandler(s, http.StatusOK, s.handleListRunState))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/stream/chunks", s.handleRunLLMStream)
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/resources", TypedHandler(s, http.StatusOK, s.handleListRunResources))
			})
		})

		r.Route("/batch-operations", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListBatchOperations))
			r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/{batchID}", TypedHandler(s, http.StatusOK, s.handleGetBatchOperation))
		})

		r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/webhook-deliveries", TypedHandler(s, http.StatusOK, s.handleListWebhookDeliveries))
		r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/webhook-deliveries/{deliveryID}/retry", TypedHandler(s, http.StatusOK, s.handleRetryWebhookDelivery))

		r.Route("/webhooks", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeRunsWrite), rateLimit(5, time.Minute)).Post("/test", TypedHandler(s, http.StatusOK, s.handleTestWebhook))
			r.Route("/deliveries", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListWebhookDeliveries))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/{id}", TypedHandler(s, http.StatusOK, s.handleGetWebhookDelivery))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/{id}/retry", TypedHandler(s, http.StatusOK, s.handleRetryWebhookDelivery))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/{id}/replay", TypedHandler(s, http.StatusCreated, s.handleReplayWebhookDelivery))
			})
			r.Route("/subscriptions", func(r chi.Router) {
				r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeRunsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateWebhookSubscription))
				r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListWebhookSubscriptions))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Delete("/{id}", TypedHandler(s, http.StatusNoContent, s.handleDeleteWebhookSubscription))
				r.With(s.requirePermission(domain.ScopeRunsWrite)).Post("/{id}/rotate-secret", TypedHandler(s, http.StatusOK, s.handleRotateWebhookSecret))
			})
		})

		r.Route("/notification-channels", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateNotificationChannel))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListNotificationChannels))
			r.Route("/{channelID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetNotificationChannel))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Patch("/", TypedHandler(s, http.StatusOK, s.handleUpdateNotificationChannel))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", TypedHandler(s, http.StatusNoContent, s.handleDeleteNotificationChannel))
			})
		})
		r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/notification-deliveries", TypedHandler(s, http.StatusOK, s.handleListNotificationDeliveries))

		// Strait Notify APIs.
		r.Route("/subscribers", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleUpsertNotifySubscriber))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListNotifySubscribers))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/{subscriberID}", TypedHandler(s, http.StatusOK, s.handleGetNotifySubscriber))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Put("/{subscriberID}", TypedHandler(s, http.StatusOK, s.handleUpdateNotifySubscriber))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/{subscriberID}", TypedHandler(s, http.StatusNoContent, s.handleDeleteNotifySubscriber))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/{subscriberID}/token", TypedHandler(s, http.StatusOK, s.handleCreateNotifySubscriberToken))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/{subscriberID}/suppressions", TypedHandler(s, http.StatusOK, s.handleListNotifySuppressionEvents))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/{subscriberID}/suppressions/unsuppress", TypedHandler(s, http.StatusOK, s.handleCreateNotifyUnsuppress))
		})

		r.Route("/topics", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateNotifyTopic))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListNotifyTopics))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/{topicKey}/subscribers", TypedHandler(s, http.StatusOK, s.handleListNotifyTopicSubscribers))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/{topicKey}/subscribers", TypedHandler(s, http.StatusNoContent, s.handleAddNotifyTopicSubscriber))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/{topicKey}/subscribers/{subscriberID}", TypedHandler(s, http.StatusNoContent, s.handleRemoveNotifyTopicSubscriber))
		})

		r.Route("/templates", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateNotificationTemplate))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListNotificationTemplates))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/{templateKey}", TypedHandler(s, http.StatusOK, s.handleGetNotificationTemplate))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Put("/{templateKey}", TypedHandler(s, http.StatusOK, s.handleUpdateNotificationTemplate))
		})

		r.Route("/categories", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateNotificationCategory))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListNotificationCategories))
		})

		r.Route("/notify/policies", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateNotifyPolicyOverride))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListNotifyPolicyOverrides))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/{policyID}", TypedHandler(s, http.StatusOK, s.handleGetNotifyPolicyOverride))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Put("/{policyID}", TypedHandler(s, http.StatusOK, s.handleUpdateNotifyPolicyOverride))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/{policyID}", TypedHandler(s, http.StatusNoContent, s.handleDeleteNotifyPolicyOverride))
		})

		r.Route("/providers", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleConfigureNotificationProvider))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListNotificationProviders))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Put("/{providerID}", TypedHandler(s, http.StatusOK, s.handleUpdateNotificationProvider))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/{providerID}", TypedHandler(s, http.StatusNoContent, s.handleDeleteNotificationProvider))
		})

		r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/notify", TypedHandler(s, http.StatusCreated, s.handleNotifyTrigger))
		r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/notify/test", TypedHandler(s, http.StatusCreated, s.handleNotifyTest))
		r.With(s.requirePermission(domain.ScopeJobsRead)).Post("/notify/preview", TypedHandler(s, http.StatusOK, s.handleNotifyPreview))
		r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/notify/deliveries", TypedHandler(s, http.StatusOK, s.handleListNotifyDeliveries))
		r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/notify/escalations/step-runs/{stepRunID}", TypedHandler(s, http.StatusOK, s.handleGetNotifyEscalationByStepRun))
		r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/notify/escalations/step-runs/{stepRunID}/acknowledge", TypedHandler(s, http.StatusOK, s.handleAcknowledgeNotifyEscalationByStepRun))
		r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/notify/escalations/step-runs/{stepRunID}/complete", TypedHandler(s, http.StatusOK, s.handleCompleteNotifyEscalationByStepRun))

		r.Route("/log-drains", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListLogDrains))
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateLogDrain))
			r.Route("/{drainID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetLogDrain))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Patch("/", TypedHandler(s, http.StatusOK, s.handleUpdateLogDrain))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", TypedHandler(s, http.StatusNoContent, s.handleDeleteLogDrain))
			})
		})

		r.Route("/api-keys", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeAPIKeysManage), httprate.LimitByIP(10, time.Minute)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateAPIKey))
			r.With(s.requirePermission(domain.ScopeAPIKeysManage)).Get("/", TypedHandler(s, http.StatusOK, s.handleListAPIKeys))
			r.With(s.requirePermission(domain.ScopeAPIKeysManage)).Get("/expiring-soon", TypedHandler(s, http.StatusOK, s.handleListExpiringKeys))
			r.With(s.requirePermission(domain.ScopeAPIKeysManage), rateLimit(10, time.Minute)).Post("/{keyID}/rotate", TypedHandler(s, http.StatusCreated, s.handleRotateAPIKey))
			r.With(s.requirePermission(domain.ScopeAPIKeysManage)).Delete("/{keyID}", TypedHandler(s, http.StatusOK, s.handleRevokeAPIKey))
		})

		r.With(s.requirePermission(domain.ScopeAPIKeysManage)).Post("/cli/device-codes/approve", TypedHandler(s, http.StatusOK, s.handleApproveDeviceCode))

		r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/stats", TypedHandler(s, http.StatusOK, s.handleStats))
		r.With(s.requirePermission(domain.ScopeRunsRead)).Post("/sse-token", TypedHandler(s, http.StatusCreated, s.handleCreateSSEToken))

		r.Route("/analytics", func(r chi.Router) {
			// Community analytics (Postgres-backed, always available)
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/performance", TypedHandler(s, http.StatusOK, s.handleGetPerformanceAnalytics))
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/costs", TypedHandler(s, http.StatusOK, s.handleGetCostAnalytics))
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/costs/trends", TypedHandler(s, http.StatusOK, s.handleGetCostTrends))
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/costs/top", TypedHandler(s, http.StatusOK, s.handleGetTopCosts))
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/compute", TypedHandler(s, http.StatusOK, s.handleGetComputeCostAnalytics))
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/approvals", TypedHandler(s, http.StatusOK, s.handleGetApprovalStats))
			r.With(s.requirePermission(domain.ScopeStatsRead)).Get("/cost-insights", TypedHandler(s, http.StatusOK, s.handleGetCostInsights))

			// Cloud-only analytics (ClickHouse-backed, requires Strait Cloud)
			r.Group(func(r chi.Router) {
				r.Use(s.requireCloudEdition)
				r.Use(s.requirePermission(domain.ScopeStatsRead))

				r.Route("/runs", func(r chi.Router) {
					r.Get("/timeline", TypedHandler(s, http.StatusOK, s.handleRunTimeline))
					r.Get("/duration-distribution", TypedHandler(s, http.StatusOK, s.handleRunDurationDistribution))
					r.Get("/failure-reasons", TypedHandler(s, http.StatusOK, s.handleRunFailureReasons))
					r.Get("/summary", TypedHandler(s, http.StatusOK, s.handleRunSummary))
					r.Get("/by-trigger", TypedHandler(s, http.StatusOK, s.handleRunsByTrigger))
				})

				r.Route("/jobs", func(r chi.Router) {
					r.Get("/comparison", TypedHandler(s, http.StatusOK, s.handleJobComparison))
					r.Get("/reliability", TypedHandler(s, http.StatusOK, s.handleJobReliability))
					r.Get("/by-version", TypedHandler(s, http.StatusOK, s.handleRunsByVersion))
					r.Get("/cost-ranking", TypedHandler(s, http.StatusOK, s.handleJobCostRanking))
					r.Get("/top-failing", TypedHandler(s, http.StatusOK, s.handleTopFailingJobs))
					r.Get("/{jobID}/history", TypedHandler(s, http.StatusOK, s.handleJobHistory))
				})

				r.Route("/tags", func(r chi.Router) {
					r.Get("/summary", TypedHandler(s, http.StatusOK, s.handleTagSummary))
					r.Get("/top-failing", TypedHandler(s, http.StatusOK, s.handleTopFailingTags))
					r.Get("/cost", TypedHandler(s, http.StatusOK, s.handleTagCost))
				})

				r.Route("/workflows", func(r chi.Router) {
					r.Get("/completion-rates", TypedHandler(s, http.StatusOK, s.handleWorkflowCompletionRates))
					r.Get("/summary", TypedHandler(s, http.StatusOK, s.handleWorkflowAnalyticsSummary))
					r.Get("/{workflowID}/step-durations", TypedHandler(s, http.StatusOK, s.handleWorkflowStepDurations))
				})

				r.Route("/webhooks", func(r chi.Router) {
					r.Get("/delivery-stats", TypedHandler(s, http.StatusOK, s.handleWebhookDeliveryStats))
					r.Get("/endpoint-health", TypedHandler(s, http.StatusOK, s.handleWebhookEndpointHealth))
					r.Get("/top-failing", TypedHandler(s, http.StatusOK, s.handleTopFailingWebhooks))
				})

				r.Route("/events", func(r chi.Router) {
					r.Get("/volume", TypedHandler(s, http.StatusOK, s.handleEventVolume))
					r.Get("/latency", TypedHandler(s, http.StatusOK, s.handleEventLatency))
				})

				r.Get("/costs/forecast", TypedHandler(s, http.StatusOK, s.handleCostForecast))
				r.Get("/costs/by-trigger", TypedHandler(s, http.StatusOK, s.handleCostByTrigger))
				r.Get("/costs/by-machine", TypedHandler(s, http.StatusOK, s.handleCostByMachine))
			})
		})

		r.Route("/roles", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeRBACManage), rateLimit(20, time.Minute)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateRole))
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/", TypedHandler(s, http.StatusOK, s.handleListRoles))
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/{roleID}", TypedHandler(s, http.StatusOK, s.handleGetRole))
			r.With(s.requirePermission(domain.ScopeRBACManage), rateLimit(20, time.Minute)).Patch("/{roleID}", TypedHandler(s, http.StatusOK, s.handleUpdateRole))
			r.With(s.requirePermission(domain.ScopeRBACManage), rateLimit(20, time.Minute)).Delete("/{roleID}", TypedHandler(s, http.StatusNoContent, s.handleDeleteRole))
		})

		r.Route("/members", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeRBACManage), rateLimit(40, time.Minute)).Post("/", TypedHandler(s, http.StatusCreated, s.handleAssignMember))
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeRBACManage), rateLimit(20, time.Minute)).Post("/bulk", TypedHandler(s, http.StatusOK, s.handleBulkAssignMembers))
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/", TypedHandler(s, http.StatusOK, s.handleListMembers))
			r.With(s.requirePermission(domain.ScopeRBACManage), rateLimit(40, time.Minute)).Delete("/{userID}", TypedHandler(s, http.StatusNoContent, s.handleRemoveMember))
		})

		r.With(s.requirePermission(domain.ScopeRBACManage), rateLimit(5, time.Minute)).Post("/seed-roles", TypedHandler(s, http.StatusOK, s.handleSeedSystemRoles))
		r.Route("/audit-events", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/", TypedHandler(s, http.StatusOK, s.handleListAuditEvents))
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/export", TypedHandler(s, http.StatusOK, s.handleExportAuditEvents))
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/verify", TypedHandler(s, http.StatusOK, s.handleVerifyAuditChain))
		})

		r.Route("/export", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/jobs", TypedHandler(s, http.StatusOK, s.handleExportJobs))
			r.With(s.requirePermission(domain.ScopeRunsRead)).Get("/runs", TypedHandler(s, http.StatusOK, s.handleExportRuns))
			r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/workflows", TypedHandler(s, http.StatusOK, s.handleExportWorkflows))
		})

		r.Route("/resource-policies", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeRBACManage)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateResourcePolicy))
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/", TypedHandler(s, http.StatusOK, s.handleListResourcePolicies))
			r.With(s.requirePermission(domain.ScopeRBACManage)).Delete("/{policyID}", TypedHandler(s, http.StatusNoContent, s.handleDeleteResourcePolicy))
		})

		r.Route("/tag-policies", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeRBACManage)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateTagPolicy))
			r.With(s.requirePermission(domain.ScopeRBACManage)).Get("/", TypedHandler(s, http.StatusOK, s.handleListTagPolicies))
			r.With(s.requirePermission(domain.ScopeRBACManage)).Delete("/{policyID}", TypedHandler(s, http.StatusNoContent, s.handleDeleteTagPolicy))
		})

		r.Route("/workflow-policies", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/{projectID}", TypedHandler(s, http.StatusOK, s.handleGetWorkflowPolicy))
			r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Put("/{projectID}", TypedHandler(s, http.StatusOK, s.handleUpsertWorkflowPolicy))
		})

		r.Route("/workflows", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateWorkflow))
			r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListWorkflows))
			r.Route("/{workflowID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetWorkflow))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Patch("/", TypedHandler(s, http.StatusOK, s.handleUpdateWorkflow))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Delete("/", TypedHandler(s, http.StatusNoContent, s.handleDeleteWorkflow))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Post("/dry-run", TypedHandler(s, http.StatusOK, s.handleDryRunWorkflow))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Post("/plan", TypedHandler(s, http.StatusOK, s.handleWorkflowPlan))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Post("/simulate", TypedHandler(s, http.StatusOK, s.handleSimulateWorkflow))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/graph", TypedHandler(s, http.StatusOK, s.handleWorkflowGraph))
				r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeWorkflowsTrigger)).Post("/trigger", TypedHandler(s, http.StatusCreated, s.handleTriggerWorkflow))
				r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/clone", TypedHandler(s, http.StatusCreated, s.handleCloneWorkflow))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/runs", TypedHandler(s, http.StatusOK, s.handleListWorkflowRuns))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/versions", TypedHandler(s, http.StatusOK, s.handleListWorkflowVersions))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/versions/{versionID}", TypedHandler(s, http.StatusOK, s.handleGetWorkflowVersion))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/versions/{versionID}/steps", TypedHandler(s, http.StatusOK, s.handleListWorkflowVersionSteps))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/versions/{fromVersionID}/diff/{toVersionID}", TypedHandler(s, http.StatusOK, s.handleWorkflowVersionDiff))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/versions/{versionID}/impact", TypedHandler(s, http.StatusOK, s.handleWorkflowVersionImpact))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/active-versions", TypedHandler(s, http.StatusOK, s.handleGetActiveVersions))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Patch("/canary", TypedHandler(s, http.StatusOK, s.handleUpdateCanaryDeployment))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/canary", TypedHandler(s, http.StatusOK, s.handleGetCanaryStatus))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/canary/rollback", TypedHandler(s, http.StatusOK, s.handleRollbackCanaryDeployment))
			})
		})

		r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/canary-deployments", TypedHandler(s, http.StatusCreated, s.handleCreateCanaryDeployment))

		r.Route("/deployments", func(r chi.Router) {
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateDeploymentVersion))
			r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListDeploymentVersions))
			r.Route("/{deploymentID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/finalize", TypedHandler(s, http.StatusOK, s.handleFinalizeDeploymentVersion))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/promote", TypedHandler(s, http.StatusOK, s.handlePromoteDeploymentVersion))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/rollback", TypedHandler(s, http.StatusOK, s.handleRollbackDeploymentVersion))
			})
		})

		r.Route("/event-sources", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListEventSources))
			r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/", TypedHandler(s, http.StatusCreated, s.handleCreateEventSource))
			r.Route("/{sourceID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetEventSource))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Patch("/", TypedHandler(s, http.StatusNoContent, s.handleUpdateEventSource))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", TypedHandler(s, http.StatusNoContent, s.handleDeleteEventSource))
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/subscriptions", TypedHandler(s, http.StatusOK, s.handleListEventSourceSubscriptions))
				r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsWrite)).Post("/subscribe", TypedHandler(s, http.StatusCreated, s.handleSubscribeToEventSource))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/subscriptions/{subID}", TypedHandler(s, http.StatusNoContent, s.handleDeleteEventSubscription))
			})
		})
		r.With(
			s.requirePermission(domain.ScopeJobsWrite),
			rateLimit(triggerRateLimitRequests, triggerRateLimitWindow),
		).Post("/events/dispatch", TypedHandler(s, http.StatusOK, s.handleDispatchEvent))

		r.Route("/events", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListEventTriggers))
			r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/stats", TypedHandler(s, http.StatusOK, s.handleGetEventTriggerStats))
			r.With(s.requirePermission(domain.ScopeJobsWrite)).Post("/purge", TypedHandler(s, http.StatusOK, s.handlePurgeEventTriggers))
			r.Route("/prefix/{prefix}", func(r chi.Router) {
				r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsTrigger), rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/send", TypedHandler(s, http.StatusOK, s.handleSendEventByPrefix))
			})
			r.Route("/{eventKey}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeJobsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetEventTrigger))
				r.With(s.requirePermission(domain.ScopeJobsWrite)).Delete("/", TypedHandler(s, http.StatusOK, s.handleCancelEventTrigger))
				r.With(s.idempotencyMiddleware, s.requirePermission(domain.ScopeJobsTrigger), rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/send", TypedHandler(s, http.StatusOK, s.handleSendEvent))
			})
		})

		r.Route("/workflow-runs", func(r chi.Router) {
			r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleListWorkflowRunsByProject))
			r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/bulk-cancel", TypedHandler(s, http.StatusOK, s.handleBulkCancelWorkflowRuns))
			r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/bulk-replay", TypedHandler(s, http.StatusOK, s.handleBulkReplayWorkflowRuns))
			r.Route("/{workflowRunID}", func(r chi.Router) {
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/", TypedHandler(s, http.StatusOK, s.handleGetWorkflowRun))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Delete("/", TypedHandler(s, http.StatusOK, s.handleCancelWorkflowRun))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/pause", TypedHandler(s, http.StatusOK, s.handlePauseWorkflowRun))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/resume", TypedHandler(s, http.StatusOK, s.handleResumeWorkflowRun))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/labels", TypedHandler(s, http.StatusOK, s.handleGetWorkflowRunLabels))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/steps", TypedHandler(s, http.StatusOK, s.handleListWorkflowStepRuns))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/graph", TypedHandler(s, http.StatusOK, s.handleGetWorkflowRunGraph))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/explain", TypedHandler(s, http.StatusOK, s.handleGetWorkflowRunExplain))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/timeline", TypedHandler(s, http.StatusOK, s.handleGetWorkflowRunTimeline))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/approve", TypedHandler(s, http.StatusOK, s.handleApproveWorkflowStep))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/skip", TypedHandler(s, http.StatusOK, s.handleSkipWorkflowStep))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/force-complete", TypedHandler(s, http.StatusOK, s.handleForceCompleteWorkflowStep))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/retry", TypedHandler(s, http.StatusOK, s.handleRetryWorkflowStep))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/steps/{stepRef}/replay-subtree", TypedHandler(s, http.StatusOK, s.handleReplayWorkflowSubtree))
				r.With(s.requirePermission(domain.ScopeWorkflowsTrigger)).Post("/retry", TypedHandler(s, http.StatusCreated, s.handleRetryWorkflowRun))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/debug", TypedHandler(s, http.StatusOK, s.handleGetWorkflowRunDebug))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/compare/{otherRunID}", TypedHandler(s, http.StatusOK, s.handleCompareWorkflowRuns))
				r.With(s.requirePermission(domain.ScopeWorkflowsWrite)).Post("/compensate", TypedHandler(s, http.StatusOK, s.handleCompensateWorkflowRun))
				r.With(s.requirePermission(domain.ScopeWorkflowsRead)).Get("/compensation-plan", TypedHandler(s, http.StatusOK, s.handleGetCompensationPlan))
			})
		})
	})

	r.Route("/sdk/v1", func(r chi.Router) {
		r.Route("/runs/{runID}", func(r chi.Router) {
			r.Use(s.runTokenAuth) // must be inside {runID} group so chi.URLParam("runID") is populated
			r.Use(s.sdkResponseHeaders)
			r.Get("/payload", TypedHandler(s, http.StatusOK, s.handleSDKGetPayload))
			r.Post("/log", TypedHandler(s, http.StatusCreated, s.handleSDKLog))
			r.Post("/progress", TypedHandler(s, http.StatusCreated, s.handleSDKProgress))
			r.Post("/annotate", TypedHandler(s, http.StatusOK, s.handleSDKAnnotate))
			r.Post("/heartbeat", TypedHandler(s, http.StatusOK, s.handleSDKHeartbeat))
			r.Post("/checkpoint", TypedHandler(s, http.StatusCreated, s.handleSDKCheckpoint))
			r.Post("/usage", TypedHandler(s, http.StatusCreated, s.handleSDKUsage))
			r.Post("/tool-call", TypedHandler(s, http.StatusCreated, s.handleSDKToolCall))
			r.Post("/output", TypedHandler(s, http.StatusCreated, s.handleSDKOutput))
			r.Post("/complete", TypedHandler(s, http.StatusOK, s.handleSDKComplete))
			r.Post("/fail", TypedHandler(s, http.StatusOK, s.handleSDKFail))
			r.Post("/spawn", TypedHandler(s, http.StatusCreated, s.handleSDKSpawn))
			r.Post("/continue", TypedHandler(s, http.StatusCreated, s.handleSDKContinue))
			r.Post("/wait-for-event", TypedHandler(s, http.StatusOK, s.handleSDKWaitForEvent))
			r.Post("/state", TypedHandler(s, http.StatusCreated, s.handleSDKSetState))
			r.Get("/state", TypedHandler(s, http.StatusOK, s.handleSDKListState))
			r.Get("/state/{key}", TypedHandler(s, http.StatusOK, s.handleSDKGetState))
			r.Delete("/state/{key}", TypedHandler(s, http.StatusNoContent, s.handleSDKDeleteState))
			r.Post("/stream", TypedHandler(s, http.StatusOK, s.handleSDKStreamChunk))
			r.Post("/resources", TypedHandler(s, http.StatusCreated, s.handleSDKResources))
			r.Post("/resource-snapshot", TypedHandler(s, http.StatusCreated, s.handleSDKResourceSnapshot))
			r.Post("/iteration", TypedHandler(s, http.StatusOK, s.handleSDKIteration))
			r.Route("/memory", func(r chi.Router) {
				r.Post("/{key}", TypedHandler(s, http.StatusCreated, s.handleSDKSetMemory))
				r.Get("/{key}", TypedHandler(s, http.StatusOK, s.handleSDKGetMemory))
				r.Get("/", TypedHandler(s, http.StatusOK, s.handleSDKListMemory))
				r.Delete("/{key}", TypedHandler(s, http.StatusNoContent, s.handleSDKDeleteMemory))
			})
		})
	})

	// API Reference
	r.Get("/reference", s.handleAPIReference)
	r.Get("/reference/openapi.json", s.handleOpenAPISpec)
	r.Get("/reference/openapi.yaml", http.RedirectHandler("/reference/openapi.json", http.StatusMovedPermanently).ServeHTTP)

	// SDK configuration schema — served publicly so IDEs and SDK CI can fetch it.
	r.Get("/schemas/v1/strait.json", s.handleStraitJSONSchema)

	return r
}
