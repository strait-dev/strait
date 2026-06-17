package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExitCodeReportsRunError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := exitCode(nil, &stdout, &stderr, os.Create)

	require.Equal(t, 1, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "usage: format-benchmarks")
}

func TestRunWritesMarkdownToStdout(t *testing.T) {
	input := writeBenchmarkInput(t, `
ignored line
BenchmarkNoPackage-8 10 0.750 ns/op
pkg: strait/internal/cache
BenchmarkLookup-8 1000 1234 ns/op 16 B/op 2 allocs/op
BenchmarkLookup-8 2000 2234 ns/op 32 B/op 4 allocs/op
pkg: github.com/acme/pkg
BenchmarkExternal 5 9.5 ns/op
BenchmarkExternal 7 10.5 ns/op
`)
	var stdout bytes.Buffer

	require.NoError(t, run([]string{input}, &stdout, os.Create))
	got := stdout.String()
	require.Contains(t, got, "### github.com/acme/pkg")
	require.Contains(t, got, "### internal/cache")
	require.Contains(t, got, "### unknown")
	require.Contains(t, got, "| Lookup | 1,500 | 1,734 | 24.0 | 3 |")
	require.Contains(t, got, "| NoPackage | 10 | 0.750 | 0 | 0 |")
	require.Contains(t, got, "**3 benchmarks** across **3 packages** | 2 iterations each")
}

func TestRunWritesMarkdownToOutputFile(t *testing.T) {
	input := writeBenchmarkInput(t, `BenchmarkOnly 1 1 ns/op`)
	output := filepath.Join(t.TempDir(), "bench.md")
	var stdout bytes.Buffer

	require.NoError(t, run([]string{input, output}, &stdout, os.Create))
	require.Empty(t, stdout.String())
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(data), "| Only | 1 | 1.0 | 0 | 0 |")
}

func TestRunReturnsCreateError(t *testing.T) {
	input := writeBenchmarkInput(t, `BenchmarkOnly 1 1 ns/op`)

	err := run([]string{input, "out.md"}, &bytes.Buffer{}, func(string) (*os.File, error) {
		return nil, errors.New("permission denied")
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "create out.md: permission denied")
}

func TestParseReturnsOpenAndScannerErrors(t *testing.T) {
	_, err := parse(filepath.Join(t.TempDir(), "missing.txt"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "open")

	longLine := bytes.Repeat([]byte("x"), 70*1024)
	path := filepath.Join(t.TempDir(), "too-long.txt")
	require.NoError(t, os.WriteFile(path, longLine, 0o600))

	_, err = parse(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read")
}

func TestWriteMarkdownHandlesEmptyAndSingleIterationResults(t *testing.T) {
	var out bytes.Buffer

	writeMarkdown(&out, map[string][]*result{
		"pkg-b": {
			{name: "Single", samples: []sample{{ops: 1, nsPerOp: 1, bPerOp: 0.25, allocs: 1}}},
		},
		"pkg-a": {},
	})

	got := out.String()
	require.Contains(t, got, "### pkg-a\n\n")
	require.Contains(t, got, "### pkg-b\n\n")
	require.Contains(t, got, "| Single | 1 | 1.0 | 0.250 | 1 |")
	require.Contains(t, got, "**1 benchmarks** across **2 packages**")
	require.NotContains(t, got, "iterations each")
}

func TestAggregateMutationEdges(t *testing.T) {
	ops, nsOp, bOp, allocs := aggregate(nil)
	require.Zero(t, ops)
	require.Zero(t, nsOp)
	require.Zero(t, bOp)
	require.Zero(t, allocs)

	ops, nsOp, bOp, allocs = aggregate([]sample{{ops: 3, nsPerOp: 0.5, bPerOp: 4, allocs: 1}})
	require.Equal(t, 3, ops)
	require.InDelta(t, 0.5, nsOp, 0)
	require.InDelta(t, 4.0, bOp, 0)
	require.Equal(t, 1, allocs)

	ops, nsOp, bOp, allocs = aggregate([]sample{
		{ops: 1, nsPerOp: 1, bPerOp: 2, allocs: 1},
		{ops: 4, nsPerOp: 3, bPerOp: 6, allocs: 4},
	})
	require.Equal(t, 3, ops)
	require.InDelta(t, 2.0, nsOp, 0)
	require.InDelta(t, 4.0, bOp, 0)
	require.Equal(t, 3, allocs)
}

func TestFormattingHelpersMutationEdges(t *testing.T) {
	require.Equal(t, "0", formatInt(0))
	require.Equal(t, "999", formatInt(999))
	require.Equal(t, "1,000", formatInt(1000))
	require.Equal(t, "12,345,678", formatInt(12345678))

	require.Equal(t, "0", formatFloat(0))
	require.Equal(t, "0.125", formatFloat(0.125))
	require.Equal(t, "1.2", formatFloat(1.25))
	require.Equal(t, "1,235", formatFloat(1234.6))

	require.Equal(t, "internal/cache", shortenPkg("strait/internal/cache"))
	require.Equal(t, "github.com/acme/pkg", shortenPkg("github.com/acme/pkg"))
	require.Equal(t, []string{"a", "b"}, sortedKeys(map[string][]*result{"b": nil, "a": nil}))
}

func writeBenchmarkInput(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "bench.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
