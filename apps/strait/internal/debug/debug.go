package debug

import (
	"log/slog"
	"net/http"
	"net/http/pprof"
	"strconv"

	"github.com/arl/statsviz"
	"github.com/go-chi/chi/v5"
)

const MaxPprofProfileSeconds = 30

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
	r.Get("/debug/pprof/profile", cappedProfile)

	for _, name := range []string{"allocs", "block", "goroutine", "mutex"} {
		r.Get("/debug/pprof/"+name, binaryProfile(name))
	}
}

func cappedProfile(w http.ResponseWriter, r *http.Request) {
	// pprof.Profile ignores the ?debug query parameter entirely (it always writes
	// the binary protobuf CPU profile), so a rejectDebugOutput guard here would be
	// dead code. The guard is only meaningful for the text-capable handlers in
	// binaryProfile.
	pprof.Profile(w, capSeconds(r, MaxPprofProfileSeconds))
}

func binaryProfile(name string) http.HandlerFunc {
	handler := pprof.Handler(name)
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectDebugOutput(w, r) {
			return
		}
		handler.ServeHTTP(w, r)
	}
}

func rejectDebugOutput(w http.ResponseWriter, r *http.Request) bool {
	for _, raw := range r.URL.Query()["debug"] {
		if raw != "" && raw != "0" {
			http.Error(w, "pprof text debug output is disabled", http.StatusBadRequest)
			return true
		}
	}
	return false
}

func capSeconds(r *http.Request, maxSeconds int) *http.Request {
	raw := r.URL.Query().Get("seconds")
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= maxSeconds {
		return r
	}

	cloned := r.Clone(r.Context())
	query := cloned.URL.Query()
	query.Set("seconds", strconv.Itoa(maxSeconds))
	cloned.URL.RawQuery = query.Encode()
	return cloned
}
