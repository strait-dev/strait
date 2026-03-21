package styles

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func init() {
	ForceNoColor()
}

var ansiPattern = regexp.MustCompile(`\x1b\[`)

func TestStatusBadge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status string
		want   string
	}{
		{"completed", "OK"},
		{"ok", "OK"},
		{"pass", "OK"},
		{"promoted", "OK"},
		{"true", "OK"},
		{"failed", "FAIL"},
		{"system_failed", "FAIL"},
		{"crashed", "FAIL"},
		{"dead_letter", "FAIL"},
		{"fail", "FAIL"},
		{"false", "FAIL"},
		{"executing", "RUN"},
		{"running", "RUN"},
		{"queued", "QUEUE"},
		{"dequeued", "QUEUE"},
		{"delayed", "PEND"},
		{"waiting", "PEND"},
		{"pending", "PEND"},
		{"canceled", "CANC"},
		{"expired", "CANC"},
		{"timed_out", "CANC"},
		{"rolled_back", "CANC"},
		{"warn", "WARN"},
		{"warning", "WARN"},
		{"unknown_xyz", "UNKNOWN_XYZ"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			t.Parallel()
			got := StatusBadge(tt.status)
			if !strings.Contains(got, tt.want) {
				t.Errorf("StatusBadge(%q) = %q, want it to contain %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestStatusBadgeCaseInsensitive(t *testing.T) {
	t.Parallel()

	got := StatusBadge("COMPLETED")
	if !strings.Contains(got, "OK") {
		t.Errorf("StatusBadge(COMPLETED) = %q, want it to contain OK", got)
	}

	got = StatusBadge("Failed")
	if !strings.Contains(got, "FAIL") {
		t.Errorf("StatusBadge(Failed) = %q, want it to contain FAIL", got)
	}
}

func TestSuccess(t *testing.T) {
	t.Parallel()
	got := Success("all good")
	if !strings.Contains(got, "\u2713") {
		t.Errorf("Success() should contain checkmark, got %q", got)
	}
	if !strings.Contains(got, "all good") {
		t.Errorf("Success() should contain message, got %q", got)
	}
}

func TestWarn(t *testing.T) {
	t.Parallel()
	got := Warn("careful")
	if !strings.Contains(got, "\u26a0") {
		t.Errorf("Warn() should contain warning symbol, got %q", got)
	}
	if !strings.Contains(got, "careful") {
		t.Errorf("Warn() should contain message, got %q", got)
	}
}

func TestErr(t *testing.T) {
	t.Parallel()
	got := Err("broken")
	if !strings.Contains(got, "\u2717") {
		t.Errorf("Err() should contain cross mark, got %q", got)
	}
	if !strings.Contains(got, "broken") {
		t.Errorf("Err() should contain message, got %q", got)
	}
}

func TestInfo(t *testing.T) {
	t.Parallel()
	got := Info("note")
	if !strings.Contains(got, "\u25cf") {
		t.Errorf("Info() should contain bullet, got %q", got)
	}
	if !strings.Contains(got, "note") {
		t.Errorf("Info() should contain message, got %q", got)
	}
}

func TestLogLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level string
		want  string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"warning", "warning"},
		{"error", "error"},
		{"fatal", "fatal"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			t.Parallel()
			got := LogLevel(tt.level)
			if !strings.Contains(got, tt.want) {
				t.Errorf("LogLevel(%q) = %q, want it to contain %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestKeyValue(t *testing.T) {
	t.Parallel()
	got := KeyValue("name", "strait")
	if !strings.Contains(got, "name:") {
		t.Errorf("KeyValue() should contain key with colon, got %q", got)
	}
	if !strings.Contains(got, "strait") {
		t.Errorf("KeyValue() should contain value, got %q", got)
	}
}

func TestSummary(t *testing.T) {
	t.Parallel()
	got := Summary(8, 2, 1)
	if !strings.Contains(got, "8 passed") {
		t.Errorf("Summary() should contain passed count, got %q", got)
	}
	if !strings.Contains(got, "2 warnings") {
		t.Errorf("Summary() should contain warned count, got %q", got)
	}
	if !strings.Contains(got, "1 failed") {
		t.Errorf("Summary() should contain failed count, got %q", got)
	}
}

func TestSummaryZeroCounts(t *testing.T) {
	t.Parallel()
	got := Summary(0, 0, 0)
	if !strings.Contains(got, "0 passed") {
		t.Errorf("Summary(0,0,0) should contain 0 passed, got %q", got)
	}
	if !strings.Contains(got, "0 warnings") {
		t.Errorf("Summary(0,0,0) should contain 0 warnings, got %q", got)
	}
	if !strings.Contains(got, "0 failed") {
		t.Errorf("Summary(0,0,0) should contain 0 failed, got %q", got)
	}
}

func TestSummaryContainsSeparators(t *testing.T) {
	t.Parallel()
	got := Summary(1, 2, 3)
	if strings.Count(got, "|") != 2 {
		t.Errorf("Summary() should contain two | separators, got %q", got)
	}
}

func TestDetailBox(t *testing.T) {
	t.Parallel()
	lines := []string{
		DetailLine("Name", "test-job"),
		DetailLine("Status", "running"),
	}
	got := DetailBox("Job Details", lines)

	if !strings.Contains(got, "\u250c") {
		t.Errorf("DetailBox() should contain top-left corner, got %q", got)
	}
	if !strings.Contains(got, "\u2514") {
		t.Errorf("DetailBox() should contain bottom-left corner, got %q", got)
	}
	if !strings.Contains(got, "\u2502") {
		t.Errorf("DetailBox() should contain vertical bar, got %q", got)
	}
	if !strings.Contains(got, "Job Details") {
		t.Errorf("DetailBox() should contain title, got %q", got)
	}
}

func TestDetailBoxEmptyLine(t *testing.T) {
	t.Parallel()
	got := DetailBox("Empty", []string{""})
	if !strings.Contains(got, "\u250c") {
		t.Errorf("DetailBox() should still have borders for empty lines, got %q", got)
	}
	if !strings.Contains(got, "\u2514") {
		t.Errorf("DetailBox() should still have bottom border, got %q", got)
	}
}

func TestDetailLine(t *testing.T) {
	t.Parallel()
	got := DetailLine("Status", "running")
	if !strings.Contains(got, "Status:") {
		t.Errorf("DetailLine() should contain label with colon, got %q", got)
	}
	if !strings.Contains(got, "running") {
		t.Errorf("DetailLine() should contain value, got %q", got)
	}
}

func TestRelativeTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"seconds", 30 * time.Second, "30s ago"},
		{"minutes", 5 * time.Minute, "5m ago"},
		{"hours", 3 * time.Hour, "3h ago"},
		{"days", 48 * time.Hour, "2d ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ts := time.Now().Add(-tt.ago)
			got := RelativeTime(ts)
			if !strings.Contains(got, tt.want) {
				t.Errorf("RelativeTime(%v ago) = %q, want it to contain %q", tt.ago, got, tt.want)
			}
		})
	}
}

func TestRelativeTimeZero(t *testing.T) {
	t.Parallel()
	got := RelativeTime(time.Time{})
	if !strings.Contains(got, "--") {
		t.Errorf("RelativeTime(zero) = %q, want it to contain --", got)
	}
}

func TestDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"milliseconds", 500 * time.Millisecond, "500ms"},
		{"seconds", 4200 * time.Millisecond, "4.2s"},
		{"minutes", 90 * time.Second, "1m30s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Duration(tt.d)
			if got != tt.want {
				t.Errorf("Duration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestEnabled(t *testing.T) {
	t.Parallel()

	t.Run("true shows badge", func(t *testing.T) {
		t.Parallel()
		got := Enabled(true)
		if !strings.Contains(got, "OK") {
			t.Errorf("Enabled(true) = %q, want it to contain OK", got)
		}
	})

	t.Run("false shows dimmed", func(t *testing.T) {
		t.Parallel()
		got := Enabled(false)
		if !strings.Contains(got, "--") {
			t.Errorf("Enabled(false) = %q, want it to contain --", got)
		}
	})
}

func TestTimestamp(t *testing.T) {
	t.Parallel()
	ts := time.Date(2025, 3, 15, 14, 30, 45, 0, time.UTC)
	got := Timestamp(ts)
	if !strings.Contains(got, "14:30:45") {
		t.Errorf("Timestamp() = %q, want it to contain 14:30:45", got)
	}
}

func TestTimestampZero(t *testing.T) {
	t.Parallel()
	got := Timestamp(time.Time{})
	if !strings.Contains(got, "--") {
		t.Errorf("Timestamp(zero) = %q, want it to contain --", got)
	}
}

func TestTimestampFull(t *testing.T) {
	t.Parallel()
	ts := time.Date(2025, 3, 15, 14, 30, 45, 0, time.UTC)
	got := TimestampFull(ts)
	if !strings.Contains(got, "2025-03-15T14:30:45Z") {
		t.Errorf("TimestampFull() = %q, want it to contain RFC3339 timestamp", got)
	}
}

func TestTimestampFullZero(t *testing.T) {
	t.Parallel()
	got := TimestampFull(time.Time{})
	if !strings.Contains(got, "--") {
		t.Errorf("TimestampFull(zero) = %q, want it to contain --", got)
	}
}

func TestDivider(t *testing.T) {
	t.Parallel()
	got := Divider()
	if !strings.Contains(got, "\u2500") {
		t.Errorf("Divider() should contain horizontal line character, got %q", got)
	}
}

func TestFilePath(t *testing.T) {
	t.Parallel()
	got := FilePath("/usr/local/bin/strait")
	if !strings.Contains(got, "/usr/local/bin/strait") {
		t.Errorf("FilePath() should contain the path, got %q", got)
	}
}

func TestResourceKind(t *testing.T) {
	t.Parallel()
	got := ResourceKind("job")
	if !strings.Contains(got, "job") {
		t.Errorf("ResourceKind() should contain the kind, got %q", got)
	}
}

func TestSectionHeader(t *testing.T) {
	t.Parallel()

	t.Run("with count", func(t *testing.T) {
		t.Parallel()
		got := SectionHeader("Queues", 5)
		if !strings.Contains(got, "Queues") {
			t.Errorf("SectionHeader() should contain title, got %q", got)
		}
		if !strings.Contains(got, "5") {
			t.Errorf("SectionHeader() should contain count, got %q", got)
		}
		if !strings.Contains(got, "===") {
			t.Errorf("SectionHeader() with count should contain === wrapper, got %q", got)
		}
	})

	t.Run("negative count omits count format", func(t *testing.T) {
		t.Parallel()
		got := SectionHeader("Overview", -1)
		if !strings.Contains(got, "Overview") {
			t.Errorf("SectionHeader() should contain title, got %q", got)
		}
	})
}

func TestStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status string
	}{
		{"completed"},
		{"failed"},
		{"system_failed"},
		{"crashed"},
		{"dead_letter"},
		{"executing"},
		{"queued"},
		{"dequeued"},
		{"delayed"},
		{"waiting"},
		{"canceled"},
		{"expired"},
		{"timed_out"},
		{"other"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			t.Parallel()
			got := Status(tt.status)
			if !strings.Contains(got, tt.status) {
				t.Errorf("Status(%q) = %q, want it to contain the status string", tt.status, got)
			}
		})
	}
}

func TestForceNoColor(t *testing.T) {
	// Not parallel -- modifies global state, but init() already called it.
	funcs := map[string]string{
		"Success":      Success("ok"),
		"Warn":         Warn("watch out"),
		"Err":          Err("bad"),
		"Info":         Info("note"),
		"StatusBadge":  StatusBadge("completed"),
		"LogLevel":     LogLevel("error"),
		"KeyValue":     KeyValue("key", "val"),
		"Summary":      Summary(1, 2, 3),
		"Divider":      Divider(),
		"Timestamp":    Timestamp(time.Now()),
		"DetailBox":    DetailBox("title", []string{"line"}),
		"DetailLine":   DetailLine("label", "value"),
		"Enabled_true": Enabled(true),
		"FilePath":     FilePath("/tmp/x"),
		"ResourceKind": ResourceKind("job"),
		"Status":       Status("completed"),
	}

	for name, output := range funcs {
		if ansiPattern.MatchString(output) {
			t.Errorf("ForceNoColor: %s still contains ANSI escapes: %q", name, output)
		}
	}
}
