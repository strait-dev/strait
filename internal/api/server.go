package api

import (
	"encoding/json"
	"net/http"

	"orchestrator/internal/config"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/queue"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	router chi.Router
	store  store.Store
	queue  queue.Queue
	pubsub pubsub.Publisher
	config *config.Config
}

func NewServer(cfg *config.Config, s store.Store, q queue.Queue, pub pubsub.Publisher) *Server {
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
	r.Use(s.requestLogger)
	r.Use(chimw.Recoverer)

	r.Get("/health", s.handleHealth)

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
		})
	})

	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
