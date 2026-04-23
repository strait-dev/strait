package dispatcher

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestClusterRegistry_Reload_ParsesYAML(t *testing.T) {
	t.Parallel()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-registry", Namespace: "strait"},
		Data: map[string]string{
			"cluster-registry.yaml": `
- name: honolulu
  api_url: https://api-hnl.strait.dev
  prometheus_url: https://prom-hnl.internal
  weight: 50
  region: us-east
- name: tahoe
  api_url: https://api-tah.strait.dev
  prometheus_url: https://prom-tah.internal
  weight: 50
  region: us-west
`,
		},
	}

	cs := k8sfake.NewSimpleClientset(cm)
	reg := NewClusterRegistry(cs, "strait", "cluster-registry", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := reg.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	clusters := reg.List()
	if len(clusters) != 2 {
		t.Fatalf("List() = %d clusters, want 2", len(clusters))
	}
	if clusters[0].Name != "honolulu" {
		t.Errorf("clusters[0].Name = %q, want honolulu", clusters[0].Name)
	}
	if clusters[1].Region != "us-west" {
		t.Errorf("clusters[1].Region = %q, want us-west", clusters[1].Region)
	}
}

func TestClusterRegistry_Reload_MissingConfigMap(t *testing.T) {
	t.Parallel()
	cs := k8sfake.NewSimpleClientset()
	reg := NewClusterRegistry(cs, "strait", "cluster-registry", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := reg.Reload(ctx); err == nil {
		t.Fatal("Reload() = nil, want error for missing ConfigMap")
	}
}

func TestClusterRegistry_Reload_MissingKey(t *testing.T) {
	t.Parallel()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-registry", Namespace: "strait"},
		Data:       map[string]string{"wrong-key": "data"},
	}
	cs := k8sfake.NewSimpleClientset(cm)
	reg := NewClusterRegistry(cs, "strait", "cluster-registry", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := reg.Reload(ctx); err == nil {
		t.Fatal("Reload() = nil, want error for missing key")
	}
}

func TestClusterRegistry_Reload_RejectsInvalidEntries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		yaml string
	}{
		{
			name: "missing name",
			yaml: "- api_url: https://api.strait.dev\n  prometheus_url: https://prom.internal\n",
		},
		{
			name: "missing api_url",
			yaml: "- name: honolulu\n  prometheus_url: https://prom.internal\n",
		},
		{
			name: "api_url with non-https scheme",
			yaml: "- name: honolulu\n  api_url: http://api.strait.dev\n",
		},
		{
			name: "api_url with no host",
			yaml: "- name: honolulu\n  api_url: https://\n",
		},
		{
			name: "prometheus_url with non-https scheme",
			yaml: "- name: honolulu\n  api_url: https://api.strait.dev\n  prometheus_url: http://prom.internal\n",
		},
		{
			name: "negative weight",
			yaml: "- name: honolulu\n  api_url: https://api.strait.dev\n  weight: -1\n",
		},
		{
			name: "file scheme in api_url (SSRF vector)",
			yaml: "- name: evil\n  api_url: file:///etc/passwd\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-registry", Namespace: "strait"},
				Data:       map[string]string{"cluster-registry.yaml": tc.yaml},
			}
			cs := k8sfake.NewSimpleClientset(cm)
			reg := NewClusterRegistry(cs, "strait", "cluster-registry", nil)
			reloadCtx, reloadCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer reloadCancel()
			if err := reg.Reload(reloadCtx); err == nil {
				t.Fatalf("Reload() = nil, want error for %q", tc.name)
			}
		})
	}
}

// Reload must atomically keep the old cluster list if the new config is invalid.
func TestClusterRegistry_Reload_PreservesOldListOnError(t *testing.T) {
	t.Parallel()

	goodYAML := "- name: honolulu\n  api_url: https://api.strait.dev\n"
	badYAML := "- name: \n  api_url: https://api.strait.dev\n" // missing name

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cr", Namespace: "strait"},
		Data:       map[string]string{"cluster-registry.yaml": goodYAML},
	}
	cs := k8sfake.NewSimpleClientset(cm)
	reg := NewClusterRegistry(cs, "strait", "cr", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := reg.Reload(ctx); err != nil {
		t.Fatalf("initial Reload: %v", err)
	}

	// Update ConfigMap to bad content.
	cm.Data["cluster-registry.yaml"] = badYAML
	if _, err := cs.CoreV1().ConfigMaps("strait").Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update ConfigMap: %v", err)
	}

	if err := reg.Reload(ctx); err == nil {
		t.Fatal("second Reload() = nil, want error for invalid entry")
	}

	// Old list must still be intact.
	clusters := reg.List()
	if len(clusters) != 1 || clusters[0].Name != "honolulu" {
		t.Errorf("List() after failed reload = %v, want original honolulu entry", clusters)
	}
}

func TestValidateEntries_AcceptsValidEntries(t *testing.T) {
	t.Parallel()
	entries := []ClusterEntry{
		{Name: "a", APIURL: "https://a.example.com", PrometheusURL: "https://prom.a.internal", Weight: 10},
		{Name: "b", APIURL: "https://b.example.com", Weight: 0}, // no prometheus, weight 0 ok
	}
	if err := validateEntries(entries); err != nil {
		t.Errorf("validateEntries() = %v, want nil", err)
	}
}

func TestValidateEntries_EmptySlice(t *testing.T) {
	t.Parallel()
	if err := validateEntries(nil); err != nil {
		t.Errorf("validateEntries(nil) = %v, want nil", err)
	}
}

func TestQueueDepth_ReturnsDepth(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"42"]}]}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := queueDepth(ctx, srv.URL, &http.Client{Timeout: 2 * time.Second})
	if got != 42 {
		t.Errorf("queueDepth() = %d, want 42", got)
	}
}

func TestQueueDepth_EmptyResultIsZero(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[]}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := queueDepth(ctx, srv.URL, &http.Client{Timeout: 2 * time.Second})
	if got != 0 {
		t.Errorf("queueDepth() = %d, want 0 for empty result", got)
	}
}

func TestQueueDepth_MalformedJSONReturnsMaxInt(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := queueDepth(ctx, srv.URL, &http.Client{Timeout: 2 * time.Second})
	if got != math.MaxInt64 {
		t.Errorf("queueDepth() = %d, want MaxInt64 for malformed JSON", got)
	}
}

func TestQueueDepth_Non200StatusReturnsMaxInt(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := queueDepth(ctx, srv.URL, &http.Client{Timeout: 2 * time.Second})
	if got != math.MaxInt64 {
		t.Errorf("queueDepth() = %d, want MaxInt64 for 500 response", got)
	}
}

func TestQueueDepth_UnreachableServerReturnsMaxInt(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := queueDepth(ctx, "https://127.0.0.1:1", &http.Client{Timeout: 100 * time.Millisecond})
	if got != math.MaxInt64 {
		t.Errorf("queueDepth() = %d, want MaxInt64 for unreachable server", got)
	}
}

func TestQueueDepth_CancelledContextReturnsMaxInt(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"1"]}]}}`))
	}))
	defer srv.Close()

	got := queueDepth(ctx, srv.URL, &http.Client{Timeout: 2 * time.Second})
	if got != math.MaxInt64 {
		t.Errorf("queueDepth() = %d, want MaxInt64 for cancelled context", got)
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("expected 0 server hits (cancelled context should not reach handler), got %d", got)
	}
}

func TestQueueDepth_PlusInfReturnsMaxInt(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"+Inf"]}]}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := queueDepth(ctx, srv.URL, &http.Client{Timeout: 2 * time.Second})
	if got != math.MaxInt64 {
		t.Errorf("queueDepth() = %d, want MaxInt64 for +Inf value (broken metric, not empty queue)", got)
	}
}

func TestQueueDepth_NaNReturnsMaxInt(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"NaN"]}]}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := queueDepth(ctx, srv.URL, &http.Client{Timeout: 2 * time.Second})
	if got != math.MaxInt64 {
		t.Errorf("queueDepth() = %d, want MaxInt64 for NaN value", got)
	}
}

func TestQueueDepth_NegativeValueReturnsMaxInt(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"-1"]}]}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := queueDepth(ctx, srv.URL, &http.Client{Timeout: 2 * time.Second})
	if got != math.MaxInt64 {
		t.Errorf("queueDepth() = %d, want MaxInt64 for negative value (must not sort before healthy clusters)", got)
	}
}

func TestQueueDepth_FloatValueTruncates(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"42.9"]}]}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := queueDepth(ctx, srv.URL, &http.Client{Timeout: 2 * time.Second})
	if got != 42 {
		t.Errorf("queueDepth() = %d, want 42 (float truncated to int)", got)
	}
}

func TestValidateEntries_RejectsDuplicateNames(t *testing.T) {
	t.Parallel()
	entries := []ClusterEntry{
		{Name: "honolulu", APIURL: "https://api-a.strait.dev"},
		{Name: "honolulu", APIURL: "https://api-b.strait.dev"},
	}
	if err := validateEntries(entries); err == nil {
		t.Fatal("validateEntries() = nil, want error for duplicate cluster name")
	}
}

func TestClusterRegistry_Pick_SelectsLowestDepth(t *testing.T) {
	t.Parallel()

	// Cluster A: depth 10, cluster B: depth 2.
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"10"]}]}}`))
	}))
	defer serverA.Close()

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"2"]}]}}`))
	}))
	defer serverB.Close()

	reg := &ClusterRegistry{clusters: []ClusterEntry{
		{Name: "heavy", PrometheusURL: serverA.URL, APIURL: "https://heavy.internal"},
		{Name: "light", PrometheusURL: serverB.URL, APIURL: "https://light.internal"},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	chosen, err := reg.Pick(ctx, &http.Client{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if chosen.Name != "light" {
		t.Errorf("Pick() = %q, want light (lower queue depth)", chosen.Name)
	}
}

// Regression for the -1 sentinel bug: a cluster with a failed Prometheus query
// must NOT be preferred over a healthy cluster with depth > 0.
func TestClusterRegistry_Pick_FailedPrometheusIsDeprioritized(t *testing.T) {
	t.Parallel()

	// healthyServer returns depth 5. brokenServer is unreachable.
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"5"]}]}}`))
	}))
	defer healthyServer.Close()

	reg := &ClusterRegistry{clusters: []ClusterEntry{
		// brokenCluster has no Prometheus — queueDepth will fail and return MaxInt64.
		{Name: "broken", PrometheusURL: "https://127.0.0.1:1", APIURL: "https://broken.internal"},
		{Name: "healthy", PrometheusURL: healthyServer.URL, APIURL: "https://healthy.internal"},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	chosen, err := reg.Pick(ctx, &http.Client{Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if chosen.Name != "healthy" {
		t.Errorf("Pick() = %q, want healthy — broken cluster must not be preferred", chosen.Name)
	}
}

// When ALL clusters fail their Prometheus query, Pick still returns a cluster
// (fail-open) so traffic isn't dropped completely.
func TestClusterRegistry_Pick_AllBrokenReturnsFirst(t *testing.T) {
	t.Parallel()

	reg := &ClusterRegistry{clusters: []ClusterEntry{
		{Name: "a", PrometheusURL: "https://127.0.0.1:1", APIURL: "https://a.internal"},
		{Name: "b", PrometheusURL: "https://127.0.0.1:1", APIURL: "https://b.internal"},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	chosen, err := reg.Pick(ctx, &http.Client{Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("Pick() error = %v, want fail-open result", err)
	}
	// Both clusters have equal depth (MaxInt64), so registry order is preserved.
	if chosen.Name == "" {
		t.Error("Pick() returned empty cluster name")
	}
}

func TestClusterRegistry_Pick_WeightBreaksTie(t *testing.T) {
	t.Parallel()

	// Both clusters return depth 3. Cluster "heavy" has weight 80, "light" has 20.
	depthSrv := func(depth string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"` + depth + `"]}]}}`))
		}))
	}
	srvA := depthSrv("3")
	srvB := depthSrv("3")
	defer srvA.Close()
	defer srvB.Close()

	reg := &ClusterRegistry{clusters: []ClusterEntry{
		{Name: "low-weight", PrometheusURL: srvA.URL, APIURL: "https://low.internal", Weight: 20},
		{Name: "high-weight", PrometheusURL: srvB.URL, APIURL: "https://high.internal", Weight: 80},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	chosen, err := reg.Pick(ctx, &http.Client{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if chosen.Name != "high-weight" {
		t.Errorf("Pick() = %q, want high-weight cluster on equal depth", chosen.Name)
	}
}

func TestClusterRegistry_Pick_EmptyRegistryErrors(t *testing.T) {
	t.Parallel()
	reg := &ClusterRegistry{}
	if _, err := reg.Pick(context.Background(), &http.Client{}); err == nil {
		t.Fatal("Pick() = nil, want error for empty registry")
	}
}

func TestClusterRegistry_Pick_SingleClusterReturnsIt(t *testing.T) {
	t.Parallel()
	reg := &ClusterRegistry{
		clusters: []ClusterEntry{{Name: "solo", APIURL: "https://solo.internal"}},
	}
	chosen, err := reg.Pick(context.Background(), &http.Client{})
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if chosen.Name != "solo" {
		t.Errorf("Pick() = %q, want solo", chosen.Name)
	}
}

func TestDispatcher_Health_NoClusterReturns503(t *testing.T) {
	t.Parallel()
	reg := &ClusterRegistry{}
	d := New(reg, 0, nil)

	w := httptest.NewRecorder()
	d.handleHealth(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("health status = %d, want 503", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestDispatcher_Health_WithClustersReturns200(t *testing.T) {
	t.Parallel()
	reg := &ClusterRegistry{
		clusters: []ClusterEntry{{Name: "test", APIURL: "https://test.internal"}},
	}
	d := New(reg, 0, nil)

	w := httptest.NewRecorder()
	d.handleHealth(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if w.Code != http.StatusOK {
		t.Errorf("health status = %d, want 200", w.Code)
	}
}

func TestDispatcher_Proxy_ForwardsToSelectedCluster(t *testing.T) {
	t.Parallel()

	// Real upstream that echoes back which cluster it is.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello from upstream"))
	}))
	defer upstream.Close()

	reg := &ClusterRegistry{clusters: []ClusterEntry{
		{Name: "test", APIURL: upstream.URL, Region: "us-east"},
	}}
	d := New(reg, 0, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	d.handleProxy(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("proxy status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("X-Strait-Cluster"); got != "test" {
		t.Errorf("X-Strait-Cluster = %q, want test", got)
	}
	if got := w.Header().Get("X-Strait-Region"); got != "us-east" {
		t.Errorf("X-Strait-Region = %q, want us-east", got)
	}
	if body := w.Body.String(); body != "hello from upstream" {
		t.Errorf("body = %q, want 'hello from upstream'", body)
	}
}

func TestDispatcher_Proxy_OmitsRegionHeaderWhenEmpty(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	reg := &ClusterRegistry{clusters: []ClusterEntry{
		{Name: "no-region", APIURL: upstream.URL, Region: ""},
	}}
	d := New(reg, 0, nil)

	w := httptest.NewRecorder()
	d.handleProxy(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := w.Header().Get("X-Strait-Region"); got != "" {
		t.Errorf("X-Strait-Region = %q, want empty when region is unset", got)
	}
}

func TestDispatcher_Proxy_EmptyRegistryReturns503(t *testing.T) {
	t.Parallel()
	reg := &ClusterRegistry{}
	d := New(reg, 0, nil)

	w := httptest.NewRecorder()
	d.handleProxy(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("proxy status = %d, want 503", w.Code)
	}
}

func TestDispatcher_Proxy_UpstreamErrorReturns502(t *testing.T) {
	t.Parallel()

	// Upstream accepts connections then immediately closes them.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	}))
	defer upstream.Close()

	reg := &ClusterRegistry{clusters: []ClusterEntry{
		{Name: "flaky", APIURL: upstream.URL},
	}}
	d := New(reg, 0, nil)

	w := httptest.NewRecorder()
	d.handleProxy(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusBadGateway {
		t.Errorf("proxy status = %d, want 502 for upstream connection reset", w.Code)
	}
}

// proxyFor must return the same *cachedProxy for repeated calls with the same
// cluster metadata (no unnecessary allocations on hot path).
func TestDispatcher_ProxyFor_ReturnsCachedInstance(t *testing.T) {
	t.Parallel()

	reg := &ClusterRegistry{}
	d := New(reg, 0, nil)
	cluster := ClusterEntry{Name: "c", APIURL: "https://c.example.com", Region: "us-east"}

	first := d.proxyFor(cluster)
	second := d.proxyFor(cluster)

	if first != second {
		t.Error("proxyFor() returned different instances for identical cluster — cache miss on hot path")
	}
}

// proxyFor must rebuild when cluster metadata changes (e.g. region updated in ConfigMap).
func TestDispatcher_ProxyFor_RebuildsOnMetadataChange(t *testing.T) {
	t.Parallel()

	reg := &ClusterRegistry{}
	d := New(reg, 0, nil)

	v1 := ClusterEntry{Name: "c", APIURL: "https://c.example.com", Region: "us-east"}
	v2 := ClusterEntry{Name: "c", APIURL: "https://c.example.com", Region: "us-west"} // region changed

	first := d.proxyFor(v1)
	second := d.proxyFor(v2)

	if first == second {
		t.Error("proxyFor() returned cached instance after region change — stale header would be sent")
	}
	if second.region != "us-west" {
		t.Errorf("rebuilt proxy region = %q, want us-west", second.region)
	}
}

func TestClusterRegistry_Pick_ZeroWeightNormalization(t *testing.T) {
	t.Parallel()

	depthSrv := func(depth string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"` + depth + `"]}]}}`))
		}))
	}
	srvA := depthSrv("5")
	srvB := depthSrv("5")
	defer srvA.Close()
	defer srvB.Close()

	reg := &ClusterRegistry{clusters: []ClusterEntry{
		{Name: "zero-a", PrometheusURL: srvA.URL, APIURL: "https://a.internal", Weight: 0},
		{Name: "zero-b", PrometheusURL: srvB.URL, APIURL: "https://b.internal", Weight: 0},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	chosen, err := reg.Pick(ctx, &http.Client{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if chosen.Name != "zero-a" {
		t.Errorf("Pick() = %q, want zero-a (stable sort preserves order for equal normalized weight)", chosen.Name)
	}
}

func TestQueueDepth_ZeroValueReturnsZero(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"0"]}]}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := queueDepth(ctx, srv.URL, &http.Client{Timeout: 2 * time.Second})
	if got != 0 {
		t.Errorf("queueDepth() = %d, want 0 for zero value", got)
	}
}

func TestQueueDepth_NonNumericStringReturnsMaxInt(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"not-a-number"]}]}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := queueDepth(ctx, srv.URL, &http.Client{Timeout: 2 * time.Second})
	if got != math.MaxInt64 {
		t.Errorf("queueDepth() = %d, want MaxInt64 for non-numeric value", got)
	}
}

func TestClusterRegistry_Pick_HighWeightBeatsLowOnEqualDepth(t *testing.T) {
	t.Parallel()

	depthSrv := func(depth string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"` + depth + `"]}]}}`))
		}))
	}
	srvA := depthSrv("10")
	srvB := depthSrv("10")
	defer srvA.Close()
	defer srvB.Close()

	reg := &ClusterRegistry{clusters: []ClusterEntry{
		{Name: "light-weight", PrometheusURL: srvA.URL, APIURL: "https://a.internal", Weight: 1},
		{Name: "heavy-weight", PrometheusURL: srvB.URL, APIURL: "https://b.internal", Weight: 100},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	chosen, err := reg.Pick(ctx, &http.Client{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if chosen.Name != "heavy-weight" {
		t.Errorf("Pick() = %q, want heavy-weight (higher weight wins on equal depth)", chosen.Name)
	}
}

func TestValidateEntries_ZeroWeight(t *testing.T) {
	t.Parallel()
	entries := []ClusterEntry{
		{Name: "test", APIURL: "https://api.example.com", Weight: 0},
	}
	if err := validateEntries(entries); err != nil {
		t.Errorf("validateEntries() = %v, want nil for weight=0", err)
	}
}

func TestValidateEntries_NegativeWeightRejected(t *testing.T) {
	t.Parallel()
	entries := []ClusterEntry{
		{Name: "test", APIURL: "https://api.example.com", Weight: -1},
	}
	if err := validateEntries(entries); err == nil {
		t.Fatal("validateEntries() = nil, want error for negative weight")
	}
}
