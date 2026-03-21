package tunnel

import (
	"testing"
)

func TestParseTunnelURL_ValidOutput(t *testing.T) {
	t.Parallel()

	output := `2024-01-15T10:00:00Z INF +--------------------------------------------------------------------------------------------+
2024-01-15T10:00:00Z INF |  Your quick Tunnel has been created! Visit it at (it may take some time to be reachable):  |
2024-01-15T10:00:00Z INF |  https://some-random-words.trycloudflare.com                                               |
2024-01-15T10:00:00Z INF +--------------------------------------------------------------------------------------------+`

	url, err := ParseTunnelURL(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://some-random-words.trycloudflare.com" {
		t.Fatalf("got %q, want https://some-random-words.trycloudflare.com", url)
	}
}

func TestParseTunnelURL_MultipleLines(t *testing.T) {
	t.Parallel()

	output := `Starting tunnel
Connecting to server
Registered tunnel connection
https://abc-def-123.trycloudflare.com
Tunnel is ready`

	url, err := ParseTunnelURL(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://abc-def-123.trycloudflare.com" {
		t.Fatalf("got %q, want https://abc-def-123.trycloudflare.com", url)
	}
}

func TestParseTunnelURL_InvalidOutput(t *testing.T) {
	t.Parallel()

	output := `Starting tunnel
Connection failed
No URL available`

	_, err := ParseTunnelURL(output)
	if err == nil {
		t.Fatal("expected error for output without tunnel URL")
	}
	if err.Error() != "no tunnel URL found in cloudflared output" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestBuildJobEndpoints_MapsCorrectly(t *testing.T) {
	t.Parallel()

	jobs := []JobEndpoint{
		{Slug: "process-payment", Path: "/jobs/payment"},
		{Slug: "send-email", Path: "/jobs/email"},
	}

	endpoints := BuildJobEndpoints("https://example.trycloudflare.com", jobs)

	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}

	want := "https://example.trycloudflare.com/jobs/payment"
	if got := endpoints["process-payment"]; got != want {
		t.Errorf("process-payment: got %q, want %q", got, want)
	}

	want = "https://example.trycloudflare.com/jobs/email"
	if got := endpoints["send-email"]; got != want {
		t.Errorf("send-email: got %q, want %q", got, want)
	}
}

func TestBuildJobEndpoints_EmptyJobs(t *testing.T) {
	t.Parallel()

	endpoints := BuildJobEndpoints("https://example.trycloudflare.com", nil)

	if len(endpoints) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(endpoints))
	}
}

func TestBuildJobEndpoints_WithSubpath(t *testing.T) {
	t.Parallel()

	jobs := []JobEndpoint{
		{Slug: "webhook-handler", Path: "/api/v1/webhooks"},
		{Slug: "health-check", Path: "health"},
	}

	endpoints := BuildJobEndpoints("https://example.trycloudflare.com", jobs)

	want := "https://example.trycloudflare.com/api/v1/webhooks"
	if got := endpoints["webhook-handler"]; got != want {
		t.Errorf("webhook-handler: got %q, want %q", got, want)
	}

	want = "https://example.trycloudflare.com/health"
	if got := endpoints["health-check"]; got != want {
		t.Errorf("health-check: got %q, want %q", got, want)
	}
}

func TestBuildJobEndpoints_DefaultPath(t *testing.T) {
	t.Parallel()

	jobs := []JobEndpoint{
		{Slug: "my-job", Path: ""},
	}

	endpoints := BuildJobEndpoints("https://example.trycloudflare.com", jobs)

	want := "https://example.trycloudflare.com/my-job"
	if got := endpoints["my-job"]; got != want {
		t.Errorf("my-job: got %q, want %q", got, want)
	}
}

func TestBuildJobEndpoints_TrailingSlashOnBase(t *testing.T) {
	t.Parallel()

	jobs := []JobEndpoint{
		{Slug: "worker", Path: "/run"},
	}

	endpoints := BuildJobEndpoints("https://example.trycloudflare.com/", jobs)

	want := "https://example.trycloudflare.com/run"
	if got := endpoints["worker"]; got != want {
		t.Errorf("worker: got %q, want %q", got, want)
	}
}
