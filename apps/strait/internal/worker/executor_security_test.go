package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"
)

func TestNewExecutor_DefaultHTTPClientBlocksPrivateDNSAtDispatch(t *testing.T) {
	restore := httputil.SetLookupHostForTest(func(host string) ([]string, error) {
		if host != "rebind.test" {
			return nil, fmt.Errorf("unexpected host lookup: %s", host)
		}
		return []string{"127.0.0.1"}, nil
	})
	t.Cleanup(restore)

	exec := NewExecutor(ExecutorConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := exec.dispatchToEndpoint(ctx, "http://rebind.test/hook", &domain.JobRun{
		ID:      "run-ssrf",
		JobID:   "job-ssrf",
		Attempt: 1,
		Payload: json.RawMessage(`{"ok":true}`),
	}, nil)
	if err == nil {
		t.Fatal("expected SSRF-safe executor client to reject private DNS answer")
		return
	}
	if !strings.Contains(err.Error(), "blocked private") && !strings.Contains(err.Error(), "resolves to private") {
		t.Fatalf("expected private-address rejection, got %v", err)
	}
}
