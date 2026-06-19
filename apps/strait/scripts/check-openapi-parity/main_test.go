package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func TestExitCodeSkipsMissingSpecsAndReportsSuccess(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := exitCode(
		&stdout,
		&stderr,
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		func(path string) (map[string]struct{}, error) {
			require.Equal(t, "internal/api/routes.go", path)
			return map[string]struct{}{"/v1/jobs": {}}, nil
		},
		func(string) (map[string]struct{}, error) {
			require.FailNow(t, "spec extractor should not run for missing specs")
			return nil, nil
		},
	)

	require.Zero(t, code)
	require.Empty(t, stderr.String())
	require.Equal(t, "OpenAPI parity check passed\n", stdout.String())
}

func TestExitCodeReportsRouteAndSpecErrors(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := exitCode(
		&stdout,
		&stderr,
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		func(string) (map[string]struct{}, error) {
			return nil, errors.New("routes failed")
		},
		nil,
	)
	require.Equal(t, 1, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "openapi parity check failed: routes failed")

	stdout.Reset()
	stderr.Reset()
	code = exitCode(
		&stdout,
		&stderr,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (map[string]struct{}, error) {
			return map[string]struct{}{"/v1/jobs": {}}, nil
		},
		func(string) (map[string]struct{}, error) {
			return nil, errors.New("spec failed")
		},
	)
	require.Equal(t, 1, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "openapi parity check failed: spec failed")
}

func TestExitCodeReportsParityMismatch(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	calls := 0

	code := exitCode(
		&stdout,
		&stderr,
		func(string) (os.FileInfo, error) { return nil, nil },
		func(string) (map[string]struct{}, error) {
			return map[string]struct{}{"/v1/jobs": {}, "/v1/runs": {}}, nil
		},
		func(path string) (map[string]struct{}, error) {
			calls++
			if path == "../../docs/openapi.yaml" {
				return map[string]struct{}{"/v1/jobs": {}, "/v1/extra": {}}, nil
			}
			return map[string]struct{}{"/v1/jobs": {}, "/v1/runs": {}}, nil
		},
	)

	require.Equal(t, 1, code)
	require.Empty(t, stderr.String())
	require.Equal(t, 2, calls)
	require.Contains(t, stdout.String(), "../../docs/openapi.yaml parity mismatch:")
	require.Contains(t, stdout.String(), "Missing paths:\n    - /v1/runs")
	require.Contains(t, stdout.String(), "Extra paths:\n    - /v1/extra")
}

func TestCompareDiffAndPathHelpers(t *testing.T) {
	var out bytes.Buffer
	routes := map[string]struct{}{"/v1/b": {}, "/v1/a": {}}
	spec := map[string]struct{}{"/v1/a": {}, "/sdk/v1/c": {}}

	require.True(t, compare(&out, "spec.yaml", routes, spec))
	require.Equal(t, []string{"/v1/b"}, diff(routes, spec))
	require.Equal(t, []string{"/sdk/v1/c"}, diff(spec, routes))
	require.Contains(t, out.String(), "spec.yaml parity mismatch")

	out.Reset()
	require.False(t, compare(&out, "spec.yaml", routes, routes))
	require.Empty(t, out.String())

	require.Empty(t, currentPrefix(nil))
	require.Equal(t, "/v1", currentPrefix([]routeScope{{prefix: "/api"}, {prefix: "/v1"}}))
	require.Equal(t, "/v1/jobs", join("", "v1/jobs/"))
	require.Equal(t, "/v1", join("/v1", "/"))
	require.Equal(t, "/v1/jobs", join("/v1/", "/jobs"))
	require.Empty(t, normalizePath(""))
	require.Equal(t, "/v1/jobs", normalizePath("v1//jobs///"))
	require.True(t, isTrackedPath("/v1/jobs"))
	require.True(t, isTrackedPath("/sdk/v1/runs"))
	require.False(t, isTrackedPath("/internal/health"))
}

func TestParseOpenAPIPathsFiltersTrackedPaths(t *testing.T) {
	got, err := parseOpenAPIPaths(strings.NewReader(`
paths:
  /v1/jobs:
    get: {}
  /sdk/v1/runs:
    post: {}
  /internal/health:
    get: {}
components:
  schemas: {}
`))
	require.NoError(t, err)
	require.Equal(t, map[string]struct{}{
		"/v1/jobs":     {},
		"/sdk/v1/runs": {},
	}, got)

	_, err = parseOpenAPIPaths(failingReader{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "read failed")
}

func TestParseRoutePathsTracksNestedRoutesAndMethods(t *testing.T) {
	got, err := parseRoutePaths(strings.NewReader(`
func routes(r chi.Router) {
  r.Get("/v1/jobs", handler)
  r.Get("/internal/health", handler)
  r.Route("/v1/projects", func(r chi.Router) {
    r.Post("/", handler)
    r.Route("/{project_id}/jobs", func(r chi.Router) {
      r.Delete("/{job_id}/", handler)
    })
  })
  r.Route("/sdk/v1", func(r chi.Router) {
    r.Handle("/runs", handler)
  })
}
`))
	require.NoError(t, err)
	require.Equal(t, map[string]struct{}{
		"/v1/jobs":     {},
		"/v1/projects": {},
		"/v1/projects/{project_id}/jobs/{job_id}": {},
		"/sdk/v1/runs": {},
	}, got)

	_, err = parseRoutePaths(failingReader{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "read failed")
}

func TestAllowedFileGuards(t *testing.T) {
	_, err := extractOpenAPIPaths("tmp/openapi.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported file:")

	_, err = extractRoutePaths("tmp/routes.go")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported file:")
}

func TestAllowedFileOpenErrors(t *testing.T) {
	_, err := extractOpenAPIPaths("internal/api/openapi.yaml")
	require.Error(t, err)

	const missingRoutePath = "missing-routes-for-test.go"
	allowedFiles[missingRoutePath] = struct{}{}
	t.Cleanup(func() { delete(allowedFiles, missingRoutePath) })

	_, err = extractRoutePaths(missingRoutePath)
	require.Error(t, err)
}

var _ io.Reader = failingReader{}
