package domain

import (
	"strings"
	"testing"
)

func TestNewVersionID_HasPrefix(t *testing.T) {
	t.Parallel()

	id := NewVersionID()
	if !strings.HasPrefix(id, VersionIDPrefix) {
		t.Fatalf("NewVersionID() = %q, want prefix %q", id, VersionIDPrefix)
	}
}

func TestNewVersionID_CorrectLength(t *testing.T) {
	t.Parallel()

	id := NewVersionID()
	expected := len(VersionIDPrefix) + VersionIDLength
	if len(id) != expected {
		t.Fatalf("len(NewVersionID()) = %d, want %d (got %q)", len(id), expected, id)
	}
}

func TestNewVersionID_Unique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool, 1000)
	for range 1000 {
		id := NewVersionID()
		if seen[id] {
			t.Fatalf("duplicate version ID: %q", id)
		}
		seen[id] = true
	}
}

func TestNewVersionID_OnlyValidChars(t *testing.T) {
	t.Parallel()

	for range 100 {
		id := NewVersionID()
		body := strings.TrimPrefix(id, VersionIDPrefix)
		for _, c := range body {
			if !strings.ContainsRune(VersionIDAlphabet, c) {
				t.Fatalf("invalid char %q in version ID %q", string(c), id)
			}
		}
	}
}
