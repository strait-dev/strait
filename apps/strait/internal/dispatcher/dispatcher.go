package dispatcher

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// Dispatcher is the main entrypoint for dispatcher mode.
// It starts an HTTP server that proxies incoming requests to the least-loaded
// Strait cluster as determined by real-time Prometheus queue depth queries.
type Dispatcher struct {
	registry *ClusterRegistry
	client   *http.Client
	port     int
	logger   *slog.Logger
}

// New creates a Dispatcher.
func New(registry *ClusterRegistry, port int, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		registry: registry,
		port:     port,
		logger:   logger,
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   2 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Run starts the dispatcher HTTP server and blocks until ctx is cancelled.
func (d *Dispatcher) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", d.handleHealth)
	mux.HandleFunc("/", d.handleProxy)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", d.port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		d.logger.Info("dispatcher listening", "port", d.port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// handleHealth responds to liveness probes from the Cloudflare LB health monitor.
// Returns 200 with a JSON body so the health endpoint is machine-readable.
func (d *Dispatcher) handleHealth(w http.ResponseWriter, _ *http.Request) {
	clusters := d.registry.List()
	w.Header().Set("Content-Type", "application/json")
	if len(clusters) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"unhealthy","reason":"no clusters in registry"}`)
		return
	}
	fmt.Fprintf(w, `{"status":"ok","clusters":%d}`, len(clusters))
}

// handleProxy picks the best cluster and reverse-proxies the request.
func (d *Dispatcher) handleProxy(w http.ResponseWriter, r *http.Request) {
	cluster, err := d.registry.Pick(r.Context(), d.client)
	if err != nil {
		d.logger.Error("dispatcher: no cluster available", "error", err)
		http.Error(w, "no upstream cluster available", http.StatusServiceUnavailable)
		return
	}

	target, err := url.Parse(cluster.APIURL)
	if err != nil {
		d.logger.Error("dispatcher: invalid cluster URL", "cluster", cluster.Name, "url", cluster.APIURL, "error", err)
		http.Error(w, "invalid upstream URL", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, e error) {
		d.logger.Warn("dispatcher: upstream error", "cluster", cluster.Name, "error", e)
		http.Error(rw, "upstream error", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("X-Strait-Cluster", cluster.Name)
		if cluster.Region != "" {
			resp.Header.Set("X-Strait-Region", cluster.Region)
		}
		return nil
	}
	// Drain and discard the original body so the transport can be reused.
	defer io.Discard.Write(nil) //nolint:errcheck

	d.logger.Debug("dispatcher: routing request", "cluster", cluster.Name, "path", r.URL.Path)
	proxy.ServeHTTP(w, r)
}
