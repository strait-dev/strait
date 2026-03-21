// Package tui provides reusable components for the interactive terminal dashboard.
package tui

import "strings"

// Binding describes a keyboard shortcut.
type Binding struct {
	Key         string
	Description string
}

// GlobalBindings returns the keyboard bindings available across all TUI views.
func GlobalBindings() []Binding {
	return []Binding{
		{Key: "?", Description: "Toggle help overlay"},
		{Key: "q / Esc", Description: "Quit (or back from detail view)"},
		{Key: "Tab", Description: "Cycle focus between panels"},
		{Key: "j / Down", Description: "Move selection down"},
		{Key: "k / Up", Description: "Move selection up"},
		{Key: "Enter", Description: "Inspect selected item"},
		{Key: "r", Description: "Refresh data"},
		{Key: "t", Description: "Trigger selected job"},
		{Key: "c", Description: "Cancel selected run"},
		{Key: "1", Description: "Switch to Runs tab"},
		{Key: "2", Description: "Switch to Jobs tab"},
		{Key: "3", Description: "Switch to Workflows tab"},
	}
}

// FormatHelp renders the help overlay text with tview color tags.
func FormatHelp() string {
	bindings := GlobalBindings()
	var b strings.Builder
	b.WriteString("[::b]Keyboard Shortcuts[::-]\n\n")

	maxKeyLen := 0
	for _, bind := range bindings {
		if len(bind.Key) > maxKeyLen {
			maxKeyLen = len(bind.Key)
		}
	}

	for _, bind := range bindings {
		padding := strings.Repeat(" ", maxKeyLen-len(bind.Key)+2)
		b.WriteString("[yellow]")
		b.WriteString(bind.Key)
		b.WriteString("[-]")
		b.WriteString(padding)
		b.WriteString(bind.Description)
		b.WriteString("\n")
	}

	b.WriteString("\n[gray]Press ? to close this help[::-]")
	return b.String()
}
