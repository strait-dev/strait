package debug

import (
	"log/slog"
	"net/http"
	"net/http/pprof"

	"github.com/arl/statsviz"
	"github.com/go-chi/chi/v5"
)

// newStatsvizServer is the constructor for a statsviz server.
// It is a package-level variable so tests can replace it to simulate errors.
var newStatsvizServer = func() (*statsviz.Server, error) {
	return statsviz.NewServer(statsviz.Root("/debug/statsviz"))
}

// MountDebugRoutes registers debug and diagnostic endpoints on the router.
// This should only be enabled in development environments.
func MountDebugRoutes(r chi.Router) {
	srv, err := newStatsvizServer()
	if err != nil {
		slog.Error("failed to create statsviz server", "error", err)
		return
	}

	r.Get("/debug/statsviz/", srv.Index())
	r.Get("/debug/statsviz/ws", srv.Ws())
}

// MountPprofRoutes registers the standard library pprof endpoints.
func MountPprofRoutes(r chi.Router) {
	r.Get("/debug/pprof", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/debug/pprof/", http.StatusMovedPermanently)
	})
	r.Get("/debug/pprof/", pprof.Index)
	r.Get("/debug/pprof/cmdline", pprof.Cmdline)
	r.Get("/debug/pprof/profile", pprof.Profile)
	r.Post("/debug/pprof/symbol", pprof.Symbol)
	r.Get("/debug/pprof/symbol", pprof.Symbol)
	r.Get("/debug/pprof/trace", pprof.Trace)

	for _, name := range []string{"allocs", "block", "goroutine", "heap", "mutex", "threadcreate"} {
		handler := pprof.Handler(name)
		r.Get("/debug/pprof/"+name, handler.ServeHTTP)
	}
}
