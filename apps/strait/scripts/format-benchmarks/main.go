package main

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var benchLineRe = regexp.MustCompile(
	`^Benchmark(\w+?)(?:-\d+)?\s+(\d+)\s+([\d.]+)\s+ns/op(?:\s+([\d.]+)\s+B/op)?(?:\s+(\d+)\s+allocs/op)?`,
)

type sample struct {
	ops     int
	nsPerOp float64
	bPerOp  float64
	allocs  int
}

type result struct {
	name    string
	samples []sample
}

type createFileFunc func(string) (*os.File, error)

func main() {
	os.Exit(exitCode(os.Args[1:], os.Stdout, os.Stderr, os.Create))
}

func exitCode(args []string, stdout io.Writer, stderr io.Writer, createFile createFileFunc) int {
	if err := run(args, stdout, createFile); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func run(args []string, stdout io.Writer, createFile createFileFunc) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: format-benchmarks <input-file> [output-file]")
	}

	pkgResults, err := parse(args[0])
	if err != nil {
		return err
	}

	out := stdout
	if len(args) >= 2 {
		file, err := createFile(args[1])
		if err != nil {
			return fmt.Errorf("create %s: %w", args[1], err)
		}
		defer file.Close()
		out = file
	}

	writeMarkdown(out, pkgResults)
	return nil
}

func parse(path string) (map[string][]*result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	pkgResults := map[string][]*result{}
	resultMap := map[string]map[string]*result{}
	var currentPkg string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		if after, found := strings.CutPrefix(line, "pkg: "); found {
			currentPkg = shortenPkg(after)
			continue
		}

		m := benchLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		name := m[1]
		ops, _ := strconv.Atoi(m[2])
		nsOp, _ := strconv.ParseFloat(m[3], 64)

		s := sample{ops: ops, nsPerOp: nsOp}
		if m[4] != "" {
			s.bPerOp, _ = strconv.ParseFloat(m[4], 64)
		}
		if m[5] != "" {
			s.allocs, _ = strconv.Atoi(m[5])
		}

		pkg := currentPkg
		if pkg == "" {
			pkg = "unknown"
		}

		if resultMap[pkg] == nil {
			resultMap[pkg] = map[string]*result{}
		}
		r, ok := resultMap[pkg][name]
		if !ok {
			r = &result{name: name}
			resultMap[pkg][name] = r
			pkgResults[pkg] = append(pkgResults[pkg], r)
		}
		r.samples = append(r.samples, s)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return pkgResults, nil
}

func writeMarkdown(out io.Writer, pkgResults map[string][]*result) {
	pkgs := sortedKeys(pkgResults)

	totalBenchmarks := 0
	for _, pkg := range pkgs {
		totalBenchmarks += len(pkgResults[pkg])
	}

	fmt.Fprintln(out, "## Benchmark Results")
	fmt.Fprintln(out)

	for _, pkg := range pkgs {
		results := pkgResults[pkg]
		fmt.Fprintf(out, "### %s\n\n", pkg)
		fmt.Fprintln(out, "| Benchmark | Iterations | ns/op | B/op | allocs/op |")
		fmt.Fprintln(out, "|:----------|-----------:|------:|-----:|----------:|")

		for _, r := range results {
			ops, nsOp, bOp, allocs := aggregate(r.samples)
			fmt.Fprintf(out, "| %s | %s | %s | %s | %s |\n",
				r.name,
				formatInt(ops),
				formatFloat(nsOp),
				formatFloat(bOp),
				formatInt(allocs),
			)
		}
		fmt.Fprintln(out)
	}

	fmt.Fprintf(out, "---\n**%d benchmarks** across **%d packages**",
		totalBenchmarks, len(pkgs))

	iterCount := 0
	for _, pkg := range pkgs {
		for _, r := range pkgResults[pkg] {
			iterCount = len(r.samples)
			break
		}
		if iterCount > 0 {
			break
		}
	}
	if iterCount > 1 {
		fmt.Fprintf(out, " | %d iterations each (showing mean)", iterCount)
	}
	fmt.Fprintln(out)
}

func aggregate(samples []sample) (ops int, nsOp, bOp float64, allocs int) {
	if len(samples) == 0 {
		return ops, nsOp, bOp, allocs
	}
	if len(samples) == 1 {
		s := samples[0]
		return s.ops, s.nsPerOp, s.bPerOp, s.allocs
	}

	var sumOps int
	var sumNs, sumB float64
	var sumAllocs int
	for _, s := range samples {
		sumOps += s.ops
		sumNs += s.nsPerOp
		sumB += s.bPerOp
		sumAllocs += s.allocs
	}
	n := float64(len(samples))
	ops = int(math.Round(float64(sumOps) / n))
	nsOp = sumNs / n
	bOp = sumB / n
	allocs = int(math.Round(float64(sumAllocs) / n))
	return ops, nsOp, bOp, allocs
}

func formatInt(v int) string {
	if v == 0 {
		return "0"
	}
	s := strconv.Itoa(v)
	if len(s) <= 3 {
		return s
	}

	var b strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		b.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func formatFloat(v float64) string {
	if v == 0 {
		return "0"
	}
	if v >= 1000 {
		return formatInt(int(math.Round(v)))
	}
	if v >= 1 {
		return strconv.FormatFloat(v, 'f', 1, 64)
	}
	return strconv.FormatFloat(v, 'f', 3, 64)
}

func shortenPkg(pkg string) string {
	if short, found := strings.CutPrefix(pkg, "strait/"); found {
		return short
	}
	return pkg
}

func sortedKeys(m map[string][]*result) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
