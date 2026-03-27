package debug

import (
	"log/slog"

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
