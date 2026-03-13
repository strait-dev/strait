package domain

import (
	"strings"
	"sync"
	"testing"
	"unicode"
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

func TestNewVersionID_NoUpperCase(t *testing.T) {
	t.Parallel()

	for range 100 {
		id := NewVersionID()
		body := strings.TrimPrefix(id, VersionIDPrefix)
		for _, c := range body {
			if unicode.IsUpper(c) {
				t.Fatalf("uppercase char %q found in version ID %q", string(c), id)
			}
		}
	}
}

func TestNewVersionID_Concurrent(t *testing.T) {
	t.Parallel()

	const goroutines = 100
	const perGoroutine = 100

	var mu sync.Mutex
	seen := make(map[string]bool, goroutines*perGoroutine)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			local := make([]string, 0, perGoroutine)
			for range perGoroutine {
				local = append(local, NewVersionID())
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range local {
				if seen[id] {
					t.Errorf("duplicate version ID: %q", id)
				}
				seen[id] = true
			}
		}()
	}
	wg.Wait()

	if len(seen) != goroutines*perGoroutine {
		t.Fatalf("expected %d unique IDs, got %d", goroutines*perGoroutine, len(seen))
	}
}

func TestNewVersionID_AlphabetConsistency(t *testing.T) {
	t.Parallel()

	// Verify the alphabet is exactly what we expect: 0-9 + a-z.
	expected := "0123456789abcdefghijklmnopqrstuvwxyz"
	if VersionIDAlphabet != expected {
		t.Fatalf("VersionIDAlphabet = %q, want %q", VersionIDAlphabet, expected)
	}
}
