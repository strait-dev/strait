package domain

import (
	"encoding/json"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

// allStatuses is the complete set of run statuses used for random selection.
var allStatuses = []RunStatus{
	StatusDelayed, StatusQueued, StatusDequeued, StatusExecuting,
	StatusWaiting, StatusCompleted, StatusFailed, StatusTimedOut,
	StatusCrashed, StatusSystemFailed, StatusCanceled, StatusExpired,
	StatusDeadLetter, StatusReplayStaged, StatusPaused,
}

// terminalStatuses are statuses with no valid outbound transitions (empty transition list).
var terminalStatuses = map[RunStatus]bool{
	StatusCompleted:    true,
	StatusFailed:       true,
	StatusTimedOut:     true,
	StatusCrashed:      true,
	StatusSystemFailed: true,
	StatusCanceled:     true,
	StatusExpired:      true,
}

// TestProperty_FSM_NoTerminalToTerminal verifies that no terminal state can
// transition to another terminal state across random transition sequences.
func TestProperty_FSM_NoTerminalToTerminal(t *testing.T) {
	t.Parallel()

	for range 2000 {
		from := allStatuses[rand.IntN(len(allStatuses))]
		to := allStatuses[rand.IntN(len(allStatuses))]

		if !terminalStatuses[from] {
			continue
		}
		if !terminalStatuses[to] {
			continue
		}

		err := ValidateTransition(from, to)
		if err == nil {
			t.Fatalf("terminal-to-terminal transition accepted: %s -> %s", from, to)
		}
	}
}

// TestProperty_HealthScore_AlwaysInRange verifies that HealthLevel returns a
// valid classification string for any HealthScore value in the float64 range.
func TestProperty_HealthScore_AlwaysInRange(t *testing.T) {
	t.Parallel()

	validLevels := map[string]bool{
		"unhealthy": true,
		"degraded":  true,
		"healthy":   true,
	}

	for range 2000 {
		// Generate scores across a wide range including negatives and values > 100.
		score := rand.Float64()*300 - 100
		h := &EndpointHealthScore{HealthScore: score}
		level := h.HealthLevel()
		if !validLevels[level] {
			t.Fatalf("HealthLevel(%f) = %q, not a valid level", score, level)
		}
	}
}

// TestProperty_Scope_WildcardAlwaysTrue verifies that a scopes list containing
// the wildcard "*" always grants access regardless of the required scope.
func TestProperty_Scope_WildcardAlwaysTrue(t *testing.T) {
	t.Parallel()

	// Generate random scope strings to ask for.
	charset := "abcdefghijklmnopqrstuvwxyz:_-."
	for range 2000 {
		length := rand.IntN(30) + 1
		buf := make([]byte, length)
		for j := range buf {
			buf[j] = charset[rand.IntN(len(charset))]
		}
		required := string(buf)
		if !HasScope([]string{ScopeAll}, required) {
			t.Fatalf("HasScope([*], %q) = false, want true", required)
		}
	}
}

// TestProperty_CronNextFireFuture verifies that parsing a valid cron expression
// always produces a next fire time strictly in the future.
func TestProperty_CronNextFireFuture(t *testing.T) {
	t.Parallel()

	// Build random but syntactically valid cron expressions.
	for range 1000 {
		minute := rand.IntN(60)
		hour := rand.IntN(24)
		dom := rand.IntN(28) + 1
		month := rand.IntN(12) + 1
		dow := rand.IntN(7)

		expr := ""
		switch rand.IntN(3) {
		case 0:
			expr = "* * * * *"
		case 1:
			expr = intToStr(minute) + " " + intToStr(hour) + " * * *"
		case 2:
			expr = intToStr(minute) + " " + intToStr(hour) + " " +
				intToStr(dom) + " " + intToStr(month) + " " + intToStr(dow)
		}

		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err := parser.Parse(expr)
		if err != nil {
			// Skip invalid expressions (e.g., dom=31 + month=2).
			continue
		}

		now := time.Now()
		next := schedule.Next(now)
		if !next.After(now) {
			t.Fatalf("cron %q: next=%v is not after now=%v", expr, next, now)
		}
	}
}

// TestProperty_FSM_ValidTransitionsAreReflected verifies that for every status
// and its declared valid targets, ValidateTransition accepts the transition.
func TestProperty_FSM_ValidTransitionsAreReflected(t *testing.T) {
	t.Parallel()

	for from, targets := range validTransitions {
		for _, to := range targets {
			if err := ValidateTransition(from, to); err != nil {
				t.Fatalf("declared valid transition %s -> %s returned error: %v", from, to, err)
			}
		}
	}
}

// TestProperty_HealthScore_BoundaryConsistency verifies that HealthLevel
// classifications are consistent with their documented thresholds across random
// scores: < 30 = unhealthy, 30-60 = degraded, > 60 = healthy.
func TestProperty_HealthScore_BoundaryConsistency(t *testing.T) {
	t.Parallel()

	for range 2000 {
		score := rand.Float64()*200 - 50
		h := &EndpointHealthScore{HealthScore: score}
		level := h.HealthLevel()

		switch {
		case score < 30:
			if level != "unhealthy" {
				t.Fatalf("score=%f: got %q, want unhealthy", score, level)
			}
		case score <= 60:
			if level != "degraded" {
				t.Fatalf("score=%f: got %q, want degraded", score, level)
			}
		default:
			if level != "healthy" {
				t.Fatalf("score=%f: got %q, want healthy", score, level)
			}
		}
	}
}

// TestProperty_Scope_EmptyScopesGrantAll verifies that an empty scopes slice
// always returns true for backwards compatibility.
func TestProperty_Scope_EmptyScopesGrantAll(t *testing.T) {
	t.Parallel()

	charset := "abcdefghijklmnopqrstuvwxyz:_-."
	for range 1000 {
		length := rand.IntN(30) + 1
		buf := make([]byte, length)
		for j := range buf {
			buf[j] = charset[rand.IntN(len(charset))]
		}
		required := string(buf)
		if !HasScope([]string{}, required) {
			t.Fatalf("HasScope([], %q) = false, want true (empty = full access)", required)
		}
	}
}

// intToStr formats an int as a string without importing strconv in the test.
func intToStr(n int) string {
	return json.Number(intToJSON(n)).String()
}

func intToJSON(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
