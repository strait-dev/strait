package tui

import (
	"strings"
	"testing"
)

func TestGlobalBindings_NotEmpty(t *testing.T) {
	t.Parallel()

	bindings := GlobalBindings()
	if len(bindings) == 0 {
		t.Fatal("expected non-empty bindings list")
	}
}

func TestGlobalBindings_AllHaveKeyAndDescription(t *testing.T) {
	t.Parallel()

	for _, b := range GlobalBindings() {
		if b.Key == "" {
			t.Fatalf("binding with empty key: %+v", b)
		}
		if b.Description == "" {
			t.Fatalf("binding %q has empty description", b.Key)
		}
	}
}

func TestFormatHelp_ContainsAllBindings(t *testing.T) {
	t.Parallel()

	help := FormatHelp()
	for _, b := range GlobalBindings() {
		if !strings.Contains(help, b.Key) {
			t.Fatalf("help text missing binding key %q", b.Key)
		}
		if !strings.Contains(help, b.Description) {
			t.Fatalf("help text missing description %q", b.Description)
		}
	}
}

func TestFormatHelp_HasHeader(t *testing.T) {
	t.Parallel()

	help := FormatHelp()
	if !strings.Contains(help, "Keyboard Shortcuts") {
		t.Fatal("help text missing header")
	}
}

func TestFormatHelp_HasCloseInstruction(t *testing.T) {
	t.Parallel()

	help := FormatHelp()
	if !strings.Contains(help, "Press ? to close") {
		t.Fatal("help text missing close instruction")
	}
}
