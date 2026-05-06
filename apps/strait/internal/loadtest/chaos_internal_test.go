//go:build loadtest

package loadtest

import (
	"os"
	"strings"
	"testing"
)

func TestChaosHarness_DoesNotUseHostWideProcessKills(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("chaos.go")
	if err != nil {
		t.Fatalf("read chaos.go: %v", err)
	}
	source := string(data)
	for _, forbidden := range []string{`"pkill"`, `"killall"`, `"pgrep"`} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("chaos harness contains host-wide process command %s", forbidden)
		}
	}
}

func TestChaosHarness_RunEventsPressureUsesCurrentSchema(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("chaos.go")
	if err != nil {
		t.Fatalf("read chaos.go: %v", err)
	}
	source := string(data)
	if strings.Contains(source, "event_type") {
		t.Fatal("run_events pressure scenario references removed event_type column")
	}
	if strings.Contains(source, "run_events (id, run_id, project_id") {
		t.Fatal("run_events pressure scenario inserts removed project_id column")
	}
	for _, required := range []string{"run_events (id, run_id, type, level, message, data, created_at)", "'loadtest_pressure'"} {
		if !strings.Contains(source, required) {
			t.Fatalf("run_events pressure scenario missing expected schema fragment %q", required)
		}
	}
}
