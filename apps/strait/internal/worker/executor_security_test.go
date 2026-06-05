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

	"github.com/stretchr/testify/require"
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
	require.Error(t,
		err)
	require.False(t,
		!strings.Contains(err.Error(), "blocked private") &&
			!strings.Contains(err.Error(), "resolves to private"))

}
