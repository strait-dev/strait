package domain

import (
	"strings"
	"sync"
	"testing"
	"unicode"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVersionID_HasPrefix(t *testing.T) {
	t.Parallel()

	id := NewVersionID()
	require.True(t,
		strings.HasPrefix(id,
			VersionIDPrefix,
		))
}

func TestNewVersionID_CorrectLength(t *testing.T) {
	t.Parallel()

	id := NewVersionID()
	expected := len(VersionIDPrefix) + VersionIDLength
	require.Len(t, id,
		expected,
	)
}

func TestNewVersionID_Unique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool, 1000)
	for range 1000 {
		id := NewVersionID()
		require.False(t,
			seen[id])

		seen[id] = true
	}
}

func TestNewVersionID_OnlyValidChars(t *testing.T) {
	t.Parallel()

	for range 100 {
		id := NewVersionID()
		body := strings.TrimPrefix(id, VersionIDPrefix)
		for _, c := range body {
			require.True(t,
				strings.ContainsRune(VersionIDAlphabet,

					c))
		}
	}
}

func TestNewVersionID_NoUpperCase(t *testing.T) {
	t.Parallel()

	for range 100 {
		id := NewVersionID()
		body := strings.TrimPrefix(id, VersionIDPrefix)
		for _, c := range body {
			require.False(t,
				unicode.
					IsUpper(c),
			)
		}
	}
}

func TestNewVersionID_Concurrent(t *testing.T) {
	t.Parallel()

	const goroutines = 100
	const perGoroutine = 100

	var mu sync.Mutex
	seen := make(map[string]bool, goroutines*perGoroutine)

	var wg conc.WaitGroup
	for range goroutines {
		wg.Go(func() {
			local := make([]string, 0, perGoroutine)
			for range perGoroutine {
				local = append(local, NewVersionID())
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range local {
				assert.False(t,
					seen[id],
				)

				seen[id] = true
			}
		})
	}
	wg.Wait()
	require.Len(t, seen,
		goroutines*
			perGoroutine,
	)
}

func TestNewVersionID_AlphabetConsistency(t *testing.T) {
	t.Parallel()

	// Verify the alphabet is exactly what we expect: 0-9 + a-z.
	expected := "0123456789abcdefghijklmnopqrstuvwxyz"
	require.Equal(t,
		expected,
		VersionIDAlphabet,
	)
}
