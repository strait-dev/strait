package debug

import (
	"log/slog"

	"github.com/arl/statsviz"
	"github.com/go-chi/chi/v5"
)

// MountDebugRoutes registers debug and diagnostic endpoints on the router.
// This should only be enabled in development environments.
func MountDebugRoutes(r chi.Router) {
	srv, err := statsviz.NewServer(statsviz.Root("/debug/statsviz"))
	if err != nil {
		slog.Error("failed to create statsviz server", "error", err)
		return
	}

	r.Get("/debug/statsviz/", srv.Index())
	r.Get("/debug/statsviz/ws", srv.Ws())
}
