package main

import (
	"fmt"
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
	output := "docs/openapi.json"
	if len(os.Args) > 1 {
		output = os.Args[1]
	}

	srv := api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:      "dump-openapi",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       "01234567890123456789012345678901",
		},
		Store:   nil,
		Queue:   nil,
		Edition: domain.EditionCloud,
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)
	srv.Close()

	if w.Code != http.StatusOK {
		fmt.Fprintf(os.Stderr, "failed to get OpenAPI spec: status %d\n%s\n", w.Code, w.Body.String())
		os.Exit(1)
	}

	if err := os.WriteFile(output, w.Body.Bytes(), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", output, err)
		os.Exit(1)
	}

	fmt.Printf("wrote OpenAPI spec to %s (%d bytes)\n", output, w.Body.Len())
}
