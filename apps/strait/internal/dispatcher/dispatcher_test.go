package dispatcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestClusterRegistry_Reload_ParsesYAML(t *testing.T) {
	t.Parallel()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-registry",
			Namespace: "strait",
		},
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

	if err := reg.Reload(context.Background()); err != nil {
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

	err := reg.Reload(context.Background())
	if err == nil {
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

	err := reg.Reload(context.Background())
	if err == nil {
		t.Fatal("Reload() = nil, want error for missing key")
	}
}

func TestClusterRegistry_Pick_SelectsLowestDepth(t *testing.T) {
	t.Parallel()

	// Two fake Prometheus endpoints: cluster A has depth 10, B has depth 2.
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"10"]}]}}`))
	}))
	defer serverA.Close()

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"result":[{"value":[0,"2"]}]}}`))
	}))
	defer serverB.Close()

	clusters := []ClusterEntry{
		{Name: "heavy", PrometheusURL: serverA.URL, APIURL: "http://heavy.internal"},
		{Name: "light", PrometheusURL: serverB.URL, APIURL: "http://light.internal"},
	}

	reg := &ClusterRegistry{clusters: clusters}
	client := &http.Client{Timeout: 2 * time.Second}

	chosen, err := reg.Pick(context.Background(), client)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if chosen.Name != "light" {
		t.Errorf("Pick() = %q, want light (lower queue depth)", chosen.Name)
	}
}

func TestClusterRegistry_Pick_EmptyRegistryErrors(t *testing.T) {
	t.Parallel()
	reg := &ClusterRegistry{}
	_, err := reg.Pick(context.Background(), &http.Client{})
	if err == nil {
		t.Fatal("Pick() = nil, want error for empty registry")
	}
}

func TestClusterRegistry_Pick_SingleClusterReturnsIt(t *testing.T) {
	t.Parallel()
	reg := &ClusterRegistry{
		clusters: []ClusterEntry{{Name: "solo", APIURL: "http://solo.internal"}},
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
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	d.handleHealth(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("health status = %d, want 503", w.Code)
	}
}

func TestDispatcher_Health_WithClustersReturns200(t *testing.T) {
	t.Parallel()
	reg := &ClusterRegistry{
		clusters: []ClusterEntry{{Name: "test", APIURL: "http://test.internal"}},
	}
	d := New(reg, 0, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	d.handleHealth(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("health status = %d, want 200", w.Code)
	}
}
