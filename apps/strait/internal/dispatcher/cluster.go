// Package dispatcher implements multi-cluster job routing for the dispatcher mode.
//
// The dispatcher reads a cluster-registry ConfigMap that lists all active Strait
// clusters with their Prometheus endpoints. On each routing decision it queries
// each cluster's queue depth and forwards the request to the cluster with the
// lowest depth. This is the component that enables geographic load balancing
// across the Honolulu and Tahoe clusters.
//
// Architecture:
//
//	Cloudflare LB (geographic steering) → Dispatcher → Cluster A (Honolulu)
//	                                                  → Cluster B (Tahoe)
//
// The dispatcher is stateless and scales horizontally. All routing decisions are
// made in-process against the Prometheus query results. No central state is shared
// between dispatcher replicas.
package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/sourcegraph/conc"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ClusterEntry describes a single Strait cluster in the registry.
type ClusterEntry struct {
	// Name is a human-readable cluster identifier (e.g. "honolulu", "tahoe").
	Name string `yaml:"name"`
	// APIURL is the base URL of the Strait API in this cluster.
	// Requests are proxied here when the cluster is selected.
	// Must use https scheme with a non-empty host.
	APIURL string `yaml:"api_url"`
	// PrometheusURL is the base URL of the Prometheus instance that scrapes
	// the cluster. Used to query queue depth for routing decisions.
	// Must use https scheme with a non-empty host.
	PrometheusURL string `yaml:"prometheus_url"`
	// Weight is an optional routing weight (1–100). Zero is treated as 1.
	// Higher weight increases the probability of selection when queue depths
	// are roughly equal (within the jitter threshold). Negative values are
	// rejected at load time.
	Weight int `yaml:"weight"`
	// Region is an optional data-centre label for logging and tracing.
	Region string `yaml:"region"`
}

// ClusterRegistry holds the live set of clusters loaded from the ConfigMap.
type ClusterRegistry struct {
	mu       sync.RWMutex
	clusters []ClusterEntry

	clientset kubernetes.Interface
	namespace string
	configmap string
	logger    *slog.Logger
}

// NewClusterRegistry creates a registry that reads from the named ConfigMap.
func NewClusterRegistry(clientset kubernetes.Interface, namespace, configmap string, logger *slog.Logger) *ClusterRegistry {
	if logger == nil {
		logger = slog.Default()
	}
	return &ClusterRegistry{
		clientset: clientset,
		namespace: namespace,
		configmap: configmap,
		logger:    logger,
	}
}

// Reload fetches the cluster-registry ConfigMap and updates the in-memory list.
// Invalid entries (missing name, invalid URL, wrong scheme, negative weight) cause
// the entire reload to be rejected — a partial bad config is worse than stale good config.
func (r *ClusterRegistry) Reload(ctx context.Context) error {
	cm, err := r.clientset.CoreV1().ConfigMaps(r.namespace).Get(ctx, r.configmap, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get cluster-registry configmap %s/%s: %w", r.namespace, r.configmap, err)
	}

	raw, ok := cm.Data["cluster-registry.yaml"]
	if !ok {
		return fmt.Errorf("configmap %s/%s missing key cluster-registry.yaml", r.namespace, r.configmap)
	}

	var clusters []ClusterEntry
	if err := yaml.Unmarshal([]byte(raw), &clusters); err != nil {
		return fmt.Errorf("parse cluster-registry: %w", err)
	}

	if err := validateEntries(clusters); err != nil {
		return fmt.Errorf("invalid cluster-registry: %w", err)
	}

	r.mu.Lock()
	r.clusters = clusters
	r.mu.Unlock()

	r.logger.Info("cluster registry reloaded", "count", len(clusters))
	return nil
}

// validateEntries checks that every ClusterEntry is usable. Returns the first
// validation error found. Validation rules:
//   - Name must be non-empty and unique within the registry
//   - APIURL must be non-empty, parseable, and use https scheme
//   - PrometheusURL, if set, must be parseable and use https scheme
//   - Weight must be >= 0
func validateEntries(clusters []ClusterEntry) error {
	seen := make(map[string]struct{}, len(clusters))
	for i, c := range clusters {
		if c.Name == "" {
			return fmt.Errorf("cluster[%d] missing name", i)
		}
		if _, dup := seen[c.Name]; dup {
			return fmt.Errorf("cluster name %q appears more than once in registry", c.Name)
		}
		seen[c.Name] = struct{}{}

		if err := validateClusterURL("api_url", c.Name, c.APIURL, true); err != nil {
			return err
		}
		if c.PrometheusURL != "" {
			if err := validateClusterURL("prometheus_url", c.Name, c.PrometheusURL, false); err != nil {
				return err
			}
		}
		if c.Weight < 0 {
			return fmt.Errorf("cluster %q has negative weight %d", c.Name, c.Weight)
		}
	}
	return nil
}

// validateClusterURL checks that a URL is non-empty (if required), parseable,
// has a non-empty host, and uses https scheme. This is a defense-in-depth check
// against misconfigured ConfigMaps causing the dispatcher to proxy to unintended
// internal endpoints.
func validateClusterURL(field, clusterName, rawURL string, required bool) error {
	if rawURL == "" {
		if required {
			return fmt.Errorf("cluster %q missing %s", clusterName, field)
		}
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("cluster %q has unparseable %s %q: %w", clusterName, field, rawURL, err)
	}
	if u.Host == "" {
		return fmt.Errorf("cluster %q has %s %q with no host", clusterName, field, rawURL)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("cluster %q has %s %q with non-https scheme %q (only https is allowed)", clusterName, field, rawURL, u.Scheme)
	}
	return nil
}

// List returns a snapshot of the current cluster list.
func (r *ClusterRegistry) List() []ClusterEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ClusterEntry, len(r.clusters))
	copy(out, r.clusters)
	return out
}

// queueDepth queries Prometheus for the total queue depth of a cluster.
//
// Return value semantics:
//   - ≥ 0: actual queue depth (0 = no queued items)
//   - math.MaxInt64: query failed or response was unparseable — cluster is
//     deprioritised but not hard-excluded from routing
//
// Separating the error sentinel from a valid 0 ensures that a cluster whose
// Prometheus is unreachable sorts last, not first.
func queueDepth(ctx context.Context, prometheusURL string, client *http.Client) int64 {
	query := `sum(strait_queue_depth{status="queued"})`
	apiURL := prometheusURL + "/api/v1/query?query=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return math.MaxInt64
	}

	resp, err := client.Do(req)
	if err != nil {
		return math.MaxInt64
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return math.MaxInt64
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return math.MaxInt64
	}

	var result struct {
		Data struct {
			Result []struct {
				Value [2]any `json:"value"` // [timestamp, "value_string"]
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		// Unparseable response — treat as unknown, not as empty.
		return math.MaxInt64
	}
	if len(result.Data.Result) == 0 {
		return 0 // no queued items → depth is genuinely 0
	}

	s, _ := result.Data.Result[0].Value[1].(string)
	// Prometheus encodes instant values as float strings (e.g. "42", "42.0").
	// Parse as float64 first to handle decimal notation and catch special
	// values (+Inf, NaN) that %d silently converts to 0, making a broken
	// metric look like an empty queue.
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return math.MaxInt64
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return math.MaxInt64
	}
	return int64(f)
}

// clusterWithDepth pairs a cluster with its current queue depth.
type clusterWithDepth struct {
	cluster ClusterEntry
	depth   int64
}

// Pick selects the cluster with the lowest queue depth.
// Clusters whose Prometheus query fails return math.MaxInt64 and sort last —
// they are included in routing as a last resort (fail-open), not preferred.
// If all clusters fail their Prometheus query they are returned in registry order.
func (r *ClusterRegistry) Pick(ctx context.Context, client *http.Client) (ClusterEntry, error) {
	clusters := r.List()
	if len(clusters) == 0 {
		return ClusterEntry{}, fmt.Errorf("cluster registry is empty")
	}
	if len(clusters) == 1 {
		return clusters[0], nil
	}

	// Query all clusters concurrently.
	results := make([]clusterWithDepth, len(clusters))
	var wg conc.WaitGroup
	for i, c := range clusters {
		wg.Go(func() {
			queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			results[i] = clusterWithDepth{
				cluster: c,
				depth:   queueDepth(queryCtx, c.PrometheusURL, client),
			}
		})
	}
	wg.Wait()

	// Sort by depth ascending (math.MaxInt64 = error, sorts last), then by
	// weight descending as tiebreaker for equal depths.
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].depth != results[j].depth {
			return results[i].depth < results[j].depth
		}
		wi := results[i].cluster.Weight
		wj := results[j].cluster.Weight
		if wi == 0 {
			wi = 1
		}
		if wj == 0 {
			wj = 1
		}
		return wi > wj
	})

	return results[0].cluster, nil
}
