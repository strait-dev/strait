package styles

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var (
	Green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	Red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	Yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	Blue   = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	Gray   = lipgloss.NewStyle().Faint(true)
	Bold   = lipgloss.NewStyle().Bold(true)
)

// Status colorizes a run/workflow status string for table output.
func Status(s string) string {
	switch s {
	case "completed":
		return Green.Render(s)
	case "failed", "system_failed", "crashed":
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

// Enabled colorizes a boolean enabled/disabled field for table output.
func Enabled(enabled bool) string {
	if enabled {
		return Green.Render("true")
	}
	return Red.Render("false")
}

// ForceNoColor disables all color output by switching to an ASCII-only profile.
func ForceNoColor() {
	lipgloss.SetColorProfile(termenv.Ascii)
}
