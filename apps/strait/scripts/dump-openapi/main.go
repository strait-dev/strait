package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"

	"strait/internal/api"
	"strait/internal/config"
	"strait/internal/domain"
)

// dump-openapi creates a minimal API server to trigger Huma's OpenAPI spec
// generation, then writes the JSON spec to the given output path.
// Usage: go run ./scripts/dump-openapi [output-path].
// Default output: docs/openapi.json.
func main() {
	os.Exit(exitCode(os.Args[1:], os.Stdout, os.Stderr, rand.Reader, os.WriteFile, fetchOpenAPISpec))
}

type writeFileFunc func(string, []byte, fs.FileMode) error

type fetchOpenAPISpecFunc func(jwtSigningKey string) (int, []byte)

func exitCode(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	random io.Reader,
	writeFile writeFileFunc,
	fetchSpec fetchOpenAPISpecFunc,
) int {
	output := "docs/openapi.json"
	if len(args) > 0 {
		output = args[0]
	}

	jwtKey := make([]byte, 32)
	if _, err := io.ReadFull(random, jwtKey); err != nil {
		fmt.Fprintf(stderr, "failed to generate JWT key: %v\n", err)
		return 1
	}

	status, body := fetchSpec(hex.EncodeToString(jwtKey))
	if status != http.StatusOK {
		fmt.Fprintf(stderr, "failed to get OpenAPI spec: status %d\n%s\n", status, string(body))
		return 1
	}

	if err := writeFile(output, body, 0o600); err != nil {
		fmt.Fprintf(stderr, "failed to write %s: %v\n", output, err)
		return 1
	}

	fmt.Fprintf(stdout, "wrote OpenAPI spec to %s (%d bytes)\n", output, len(body))
	return 0
}

func fetchOpenAPISpec(jwtSigningKey string) (int, []byte) {
	srv := api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:      "dump-openapi-placeholder",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       jwtSigningKey,
		},
		Store:   nil,
		Queue:   nil,
		Edition: domain.EditionCloud,
	})
	defer srv.Close()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)
	return w.Code, w.Body.Bytes()
}
