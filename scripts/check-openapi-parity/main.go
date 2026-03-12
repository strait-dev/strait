package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var (
	routeDeclRe = regexp.MustCompile(`r\.Route\("([^"]+)"`)
	methodRe    = regexp.MustCompile(`\.((Get|Post|Patch|Delete|Put|Options|Head|Handle))\("([^"]+)"`)
	opPathRe    = regexp.MustCompile(`^\s{2}(/[^:]+):\s*$`)
)

type routeScope struct {
	prefix string
	depth  int
}

func main() {
	routePaths, err := extractRoutePaths("internal/api/routes.go")
	if err != nil {
		fatal(err)
	}
	docsPaths, err := extractOpenAPIPaths("docs/openapi.yaml")
	if err != nil {
		fatal(err)
	}
	internalPaths, err := extractOpenAPIPaths("internal/api/openapi.yaml")
	if err != nil {
		fatal(err)
	}

	if fail := compare("docs/openapi.yaml", routePaths, docsPaths); fail {
		os.Exit(1)
	}
	if fail := compare("internal/api/openapi.yaml", routePaths, internalPaths); fail {
		os.Exit(1)
	}

	fmt.Println("OpenAPI parity check passed")
}

func compare(name string, routes, openapi map[string]struct{}) bool {
	missing := diff(routes, openapi)
	extra := diff(openapi, routes)
	if len(missing) == 0 && len(extra) == 0 {
		return false
	}

	fmt.Printf("\n%s parity mismatch:\n", name)
	if len(missing) > 0 {
		fmt.Println("  Missing paths:")
		for _, p := range missing {
			fmt.Printf("    - %s\n", p)
		}
	}
	if len(extra) > 0 {
		fmt.Println("  Extra paths:")
		for _, p := range extra {
			fmt.Printf("    - %s\n", p)
		}
	}
	return true
}

func diff(a, b map[string]struct{}) []string {
	out := make([]string, 0)
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

var allowedFiles = map[string]struct{}{
	"internal/api/routes.go":    {},
	"docs/openapi.yaml":         {},
	"internal/api/openapi.yaml": {},
}

func extractOpenAPIPaths(path string) (map[string]struct{}, error) {
	if _, ok := allowedFiles[path]; !ok {
		return nil, fmt.Errorf("unsupported file: %s", path)
	}
	f, err := os.Open(path) // #nosec G304 -- path is restricted by allowedFiles map above
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]struct{}{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		m := opPathRe.FindStringSubmatch(line)
		if len(m) == 0 {
			continue
		}
		p := strings.TrimSpace(m[1])
		if isTrackedPath(p) {
			out[p] = struct{}{}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func extractRoutePaths(path string) (map[string]struct{}, error) {
	if _, ok := allowedFiles[path]; !ok {
		return nil, fmt.Errorf("unsupported file: %s", path)
	}
	f, err := os.Open(path) // #nosec G304 -- path is restricted by allowedFiles map above
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]struct{}{}
	scopes := []routeScope{}
	depth := 0

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()

		for len(scopes) > 0 && depth < scopes[len(scopes)-1].depth {
			scopes = scopes[:len(scopes)-1]
		}

		if m := routeDeclRe.FindStringSubmatch(line); len(m) > 0 {
			prefix := currentPrefix(scopes)
			scopes = append(scopes, routeScope{prefix: join(prefix, m[1]), depth: depth + 1})
		}

		if m := methodRe.FindStringSubmatch(line); len(m) > 0 {
			prefix := currentPrefix(scopes)
			p := join(prefix, m[3])
			if isTrackedPath(p) {
				out[p] = struct{}{}
			}
		}

		depth += strings.Count(line, "{")
		depth -= strings.Count(line, "}")
		if depth < 0 {
			depth = 0
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func currentPrefix(scopes []routeScope) string {
	if len(scopes) == 0 {
		return ""
	}
	return scopes[len(scopes)-1].prefix
}

func join(a, b string) string {
	if a == "" {
		return normalizePath(b)
	}
	if b == "/" {
		return normalizePath(a)
	}
	return normalizePath(strings.TrimRight(a, "/") + "/" + strings.TrimLeft(b, "/"))
}

func normalizePath(p string) string {
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

func isTrackedPath(p string) bool {
	return strings.HasPrefix(p, "/v1/") || strings.HasPrefix(p, "/sdk/v1/")
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "openapi parity check failed: %v\n", err)
	os.Exit(1)
}
