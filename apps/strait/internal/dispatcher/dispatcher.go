package dispatcher

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

// cachedProxy bundles a pre-built ReverseProxy with the cluster metadata needed
// to set response headers. Built once per unique APIURL and reused across requests.
type cachedProxy struct {
	proxy  *httputil.ReverseProxy
	name   string
	region string
}

// Dispatcher is the main entrypoint for dispatcher mode.
// It starts an HTTP server that proxies incoming requests to the least-loaded
// Strait cluster as determined by real-time Prometheus queue depth queries.
type Dispatcher struct {
	registry   *ClusterRegistry
	client     *http.Client
	port       int
	logger     *slog.Logger
	proxyCache sync.Map // map[apiURL string]*cachedProxy
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
// The ReverseProxy for each cluster is cached by APIURL to avoid per-request
// allocations. The cache is invalidated lazily: if the cluster name or region
// recorded in the cache differs from the currently selected cluster, the entry
// is rebuilt.
func (d *Dispatcher) handleProxy(w http.ResponseWriter, r *http.Request) {
	cluster, err := d.registry.Pick(r.Context(), d.client)
	if err != nil {
		d.logger.Error("dispatcher: no cluster available", "error", err)
		http.Error(w, "no upstream cluster available", http.StatusServiceUnavailable)
		return
	}

	cp := d.proxyFor(cluster)

	d.logger.Debug("dispatcher: routing request", "cluster", cluster.Name, "path", r.URL.Path)
	cp.proxy.ServeHTTP(w, r)
}

// proxyFor returns a cached *cachedProxy for the cluster, building one if needed
// or if the cluster metadata has changed since the last build.
func (d *Dispatcher) proxyFor(cluster ClusterEntry) *cachedProxy {
	if v, ok := d.proxyCache.Load(cluster.APIURL); ok {
		cp := v.(*cachedProxy)
		// Rebuild if name or region changed (e.g. after a ConfigMap reload).
		if cp.name == cluster.Name && cp.region == cluster.Region {
			return cp
		}
	}

	target, err := url.Parse(cluster.APIURL)
	if err != nil {
		// APIURL is validated at Reload time; this path is unreachable in normal
		// operation. Return a proxy that immediately returns 502.
		d.logger.Error("dispatcher: invalid cluster URL (should have been caught at load)", "cluster", cluster.Name, "url", cluster.APIURL, "error", err)
		proxy := &httputil.ReverseProxy{
			Director: func(_ *http.Request) {},
			ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, _ error) {
				http.Error(rw, "invalid upstream URL", http.StatusInternalServerError)
			},
		}
		return &cachedProxy{proxy: proxy, name: cluster.Name, region: cluster.Region}
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	name := cluster.Name
	region := cluster.Region
	proxy.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, e error) {
		d.logger.Warn("dispatcher: upstream error", "cluster", name, "error", e)
		http.Error(rw, "upstream error", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("X-Strait-Cluster", name)
		if region != "" {
			resp.Header.Set("X-Strait-Region", region)
		}
		return nil
	}

	cp := &cachedProxy{proxy: proxy, name: name, region: region}
	d.proxyCache.Store(cluster.APIURL, cp)
	return cp
}
