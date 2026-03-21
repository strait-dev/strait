// Package styles provides a rich visual style system for CLI output.
// Inspired by github.com/leonardomso/gone's UI patterns: colored badges,
// detail panels with box-drawing borders, semantic message prefixes,
// and structured formatting.
//
// All styled output is gated behind IsTTYRich() which returns false for
// --json, --yaml, --csv, --quiet, --no-color, and non-TTY environments.
package styles

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Color palette.
var (
	PrimaryColor   = lipgloss.Color("39")  // Bright blue (Strait brand)
	SecondaryColor = lipgloss.Color("241") // Gray
	SuccessColor   = lipgloss.Color("82")  // Green
	ErrorColor     = lipgloss.Color("196") // Red
	WarningColor   = lipgloss.Color("214") // Orange
	InfoColor      = lipgloss.Color("75")  // Light blue
	MutedColor     = lipgloss.Color("245") // Dimmed
	AccentColor    = lipgloss.Color("213") // Pink
)

// Text styles.
var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor)

	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("252"))

	LabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(SecondaryColor)

	ValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	MutedStyle = lipgloss.NewStyle().
			Foreground(MutedColor)

	SelectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor)

	// Legacy aliases for backwards compatibility.
	Green  = lipgloss.NewStyle().Foreground(SuccessColor)
	Red    = lipgloss.NewStyle().Foreground(ErrorColor)
	Yellow = lipgloss.NewStyle().Foreground(WarningColor)
	Blue   = lipgloss.NewStyle().Foreground(InfoColor)
	Gray   = MutedStyle
	Bold   = lipgloss.NewStyle().Bold(true)
)

// Badge styles with colored backgrounds.
var (
	BadgeOK = lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(SuccessColor).
		Padding(0, 1).
		Bold(true)

	BadgeFail = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(ErrorColor).
			Padding(0, 1).
			Bold(true)

	BadgeWarn = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(WarningColor).
			Padding(0, 1).
			Bold(true)

	BadgeRunning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("226")). // Bright yellow
			Padding(0, 1).
			Bold(true)

	BadgeQueued = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(InfoColor).
			Padding(0, 1).
			Bold(true)

	BadgePending = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(SecondaryColor).
			Padding(0, 1).
			Bold(true)

	BadgeCanceled = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(MutedColor).
			Padding(0, 1).
			Bold(true)
)

// StatusBadge returns a colored badge for a run/workflow/deployment status.
func StatusBadge(status string) string {
	switch strings.ToLower(status) {
	case "completed", "ok", "pass", "promoted", "true":
		return BadgeOK.Render("OK")
	case "failed", "system_failed", "crashed", "dead_letter", "fail", "false":
		return BadgeFail.Render("FAIL")
	case "executing", "running":
		return BadgeRunning.Render("RUN")
	case "queued", "dequeued":
		return BadgeQueued.Render("QUEUE")
	case "delayed", "waiting", "pending":
		return BadgePending.Render("PEND")
	case "canceled", "expired", "timed_out", "rolled_back":
		return BadgeCanceled.Render("CANC")
	case "warn", "warning":
		return BadgeWarn.Render("WARN")
	default:
		return BadgePending.Render(strings.ToUpper(status))
	}
}

// Status colorizes a status string with foreground color (for inline use in tables).
func Status(s string) string {
	switch s {
	case "completed":
		return Green.Render(s)
	case "failed", "system_failed", "crashed", "dead_letter":
		return Red.Render(s)
	case "executing", "queued", "dequeued":
		return Yellow.Render(s)
	case "delayed", "waiting":
		return Blue.Render(s)
	case "canceled", "expired", "timed_out":
		return Gray.Render(s)
	default:
		return s
	}
}

// Enabled returns a colored badge for boolean enabled/disabled states.
func Enabled(enabled bool) string {
	if enabled {
		return BadgeOK.Render("OK")
	}
	return MutedStyle.Render("--")
}

// Semantic message prefixes.

// Success returns a green success message with checkmark.
func Success(msg string) string {
	return lipgloss.NewStyle().Foreground(SuccessColor).Render("✓") + " " + msg
}

// Warn returns an orange warning message.
func Warn(msg string) string {
	return lipgloss.NewStyle().Foreground(WarningColor).Render("⚠") + " " + msg
}

// Err returns a red error message.
func Err(msg string) string {
	return lipgloss.NewStyle().Foreground(ErrorColor).Render("✗") + " " + msg
}

// Info returns a blue informational message.
func Info(msg string) string {
	return lipgloss.NewStyle().Foreground(InfoColor).Render("●") + " " + msg
}

// LogLevel colorizes a log level string.
func LogLevel(level string) string {
	switch strings.ToLower(level) {
	case "debug":
		return MutedStyle.Render(level)
	case "info":
		return lipgloss.NewStyle().Foreground(InfoColor).Render(level)
	case "warn", "warning":
		return lipgloss.NewStyle().Foreground(WarningColor).Render(level)
	case "error", "fatal":
		return lipgloss.NewStyle().Foreground(ErrorColor).Render(level)
	default:
		return level
	}
}

// Layout helpers.

// KeyValue renders a labeled value with dimmed key.
func KeyValue(key, value string) string {
	return LabelStyle.Render(fmt.Sprintf("  %-14s", key+":")) + " " + value
}

// SectionHeader renders a bold section header with optional count.
func SectionHeader(title string, count int) string {
	if count >= 0 {
		return HeaderStyle.Render(fmt.Sprintf("=== %s (%d) ===", title, count))
	}
	return HeaderStyle.Render(title)
}

// Summary renders a colored summary line: "8 passed | 2 warnings | 1 failed".
func Summary(passed, warned, failed int) string {
	parts := []string{
		lipgloss.NewStyle().Foreground(SuccessColor).Render(fmt.Sprintf("✓ %d passed", passed)),
		lipgloss.NewStyle().Foreground(WarningColor).Render(fmt.Sprintf("⚠ %d warnings", warned)),
		lipgloss.NewStyle().Foreground(ErrorColor).Render(fmt.Sprintf("✗ %d failed", failed)),
	}
	return strings.Join(parts, " | ")
}

// Divider renders a dimmed horizontal line.
func Divider() string {
	return MutedStyle.Render("─────────────────────────────────────────")
}

// Timestamp renders a dimmed timestamp.
func Timestamp(t time.Time) string {
	if t.IsZero() {
		return MutedStyle.Render("--")
	}
	return MutedStyle.Render(t.Format("15:04:05"))
}

// TimestampFull renders a dimmed full timestamp.
func TimestampFull(t time.Time) string {
	if t.IsZero() {
		return MutedStyle.Render("--")
	}
	return MutedStyle.Render(t.Format(time.RFC3339))
}

// RelativeTime renders a human-friendly relative time like "2m ago" or "3d ago".
func RelativeTime(t time.Time) string {
	if t.IsZero() {
		return MutedStyle.Render("--")
	}
	d := time.Since(t)
	var s string
	switch {
	case d < time.Minute:
		s = fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		s = fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		s = fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		s = fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return MutedStyle.Render(s)
}

// Duration renders a human-friendly duration like "4.2s" or "1m30s".
func Duration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

// FilePath renders a dimmed file path.
func FilePath(path string) string {
	return MutedStyle.Render(path)
}

// ResourceKind renders a colored resource type label.
func ResourceKind(kind string) string {
	return lipgloss.NewStyle().Foreground(AccentColor).Bold(true).Render(kind)
}

// Detail panel with box-drawing borders.

// DetailBox renders a bordered detail panel.
func DetailBox(title string, lines []string) string {
	width := 50
	for _, line := range lines {
		if len(line)+4 > width {
			width = len(line) + 4
		}
	}
	if len(title)+6 > width {
		width = len(title) + 6
	}

	border := MutedStyle.Render
	var b strings.Builder

	// Top border
	b.WriteString(border("┌─ "))
	b.WriteString(HeaderStyle.Render(title))
	b.WriteString(border(" "))
	remaining := width - len(title) - 4
	if remaining > 0 {
		b.WriteString(border(strings.Repeat("─", remaining)))
	}
	b.WriteString("\n")

	// Content lines
	for _, line := range lines {
		if line == "" {
			b.WriteString(border("│") + "\n")
		} else {
			b.WriteString(border("│") + " " + line + "\n")
		}
	}

	// Bottom border
	b.WriteString(border("└" + strings.Repeat("─", width)))
	b.WriteString("\n")

	return b.String()
}

// DetailLine renders a label: value pair for use inside DetailBox.
func DetailLine(label, value string) string {
	return LabelStyle.Render(fmt.Sprintf("%-12s", label+":")) + " " + value
}

// ForceNoColor disables all color output by switching to an ASCII-only profile.
func ForceNoColor() {
	lipgloss.SetColorProfile(termenv.Ascii)
}
