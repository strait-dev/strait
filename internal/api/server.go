package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"orchestrator/internal/config"
	"orchestrator/internal/domain"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/queue"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/riandyrn/otelchi"
)

// APIStore is the subset of store operations needed by the API handlers.
type APIStore interface {
	CreateJob(ctx context.Context, job *domain.Job) error
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error)
	ListJobs(ctx context.Context, projectID string) ([]domain.Job, error)
	UpdateJob(ctx context.Context, job *domain.Job) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, limit int, cursor *time.Time) ([]domain.JobRun, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	ListChildRuns(ctx context.Context, parentRunID string) ([]domain.JobRun, error)
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	UpdateHeartbeat(ctx context.Context, id string) error
	QueueStats(ctx context.Context) (*store.QueueStats, error)
}

type Server struct {
	router chi.Router
	store  APIStore
	queue  queue.Queue
	pubsub pubsub.Publisher
	config *config.Config
}

func NewServer(cfg *config.Config, s APIStore, q queue.Queue, pub pubsub.Publisher) *Server {
	srv := &Server{
		store:  s,
		queue:  q,
		pubsub: pub,
		config: cfg,
	}
	srv.router = srv.routes()
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) routes() chi.Router {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(otelchi.Middleware("orchestrator", otelchi.WithChiRoutes(r)))
	r.Use(s.requestLogger)
	r.Use(chimw.Recoverer)

	r.Get("/health", s.handleHealth)
	r.Get("/health/ready", s.handleHealthReady)

	r.Route("/v1", func(r chi.Router) {
		r.Use(s.internalSecretAuth)

		r.Route("/jobs", func(r chi.Router) {
			r.Post("/", s.handleCreateJob)
			r.Get("/", s.handleListJobs)

			r.Route("/{jobID}", func(r chi.Router) {
				r.Get("/", s.handleGetJob)
				r.Patch("/", s.handleUpdateJob)
				r.Delete("/", s.handleDeleteJob)
				r.Post("/trigger", s.handleTriggerJob)
			})
		})

		r.Route("/runs", func(r chi.Router) {
			r.Get("/", s.handleListRuns)
			r.Route("/{runID}", func(r chi.Router) {
				r.Get("/", s.handleGetRun)
				r.Delete("/", s.handleCancelRun)
				r.Get("/stream", s.handleRunStream)
				r.Get("/children", s.handleListChildRuns)
			})
		})

		r.Get("/stats", s.handleStats)
	})

	r.Route("/sdk/v1", func(r chi.Router) {
		r.Use(s.runTokenAuth)
		r.Route("/runs/{runID}", func(r chi.Router) {
			r.Post("/log", s.handleSDKLog)
			r.Post("/heartbeat", s.handleSDKHeartbeat)
			r.Post("/complete", s.handleSDKComplete)
			r.Post("/fail", s.handleSDKFail)
			r.Post("/spawn", s.handleSDKSpawn)
		})
	})

	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleHealthReady(w http.ResponseWriter, r *http.Request) {
	// Verify database connectivity via a lightweight query
	_, err := s.store.QueueStats(r.Context())
	if err != nil {
		respondError(w, http.StatusServiceUnavailable, "database not ready")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// validateURL checks that a URL is valid and doesn't target private networks.
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must use http or https scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	// Block private/internal IPs (SSRF protection)
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("URL must not point to private or loopback addresses")
		}
	}

	return nil
}
