package main

import (
	"bytes"
	"errors"
	"io/fs"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("random failed")
}

func TestExitCodeWritesDefaultOpenAPIPath(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var gotPath string
	var gotMode fs.FileMode
	var gotBody []byte

	code := exitCode(
		nil,
		&stdout,
		&stderr,
		bytes.NewReader(bytes.Repeat([]byte{0xab}, 32)),
		func(path string, body []byte, mode fs.FileMode) error {
			gotPath = path
			gotMode = mode
			gotBody = append([]byte(nil), body...)
			return nil
		},
		func(jwtSigningKey string) (int, []byte) {
			require.Equal(t, "abababababababababababababababababababababababababababababababab", jwtSigningKey)
			return http.StatusOK, []byte(`{"openapi":"3.1.0"}`)
		},
	)

	require.Zero(t, code)
	require.Empty(t, stderr.String())
	require.Equal(t, "docs/openapi.json", gotPath)
	require.Equal(t, fs.FileMode(0o600), gotMode)
	require.JSONEq(t, `{"openapi":"3.1.0"}`, string(gotBody))
	require.Contains(t, stdout.String(), "wrote OpenAPI spec to docs/openapi.json")
	require.Contains(t, stdout.String(), "19 bytes")
}

func TestExitCodeUsesExplicitOutputPath(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var gotPath string

	code := exitCode(
		[]string{"tmp/openapi.json"},
		&stdout,
		&stderr,
		bytes.NewReader(bytes.Repeat([]byte{0x01}, 32)),
		func(path string, _ []byte, _ fs.FileMode) error {
			gotPath = path
			return nil
		},
		func(string) (int, []byte) {
			return http.StatusOK, []byte(`{}`)
		},
	)

	require.Zero(t, code)
	require.Empty(t, stderr.String())
	require.Equal(t, "tmp/openapi.json", gotPath)
	require.Contains(t, stdout.String(), "tmp/openapi.json")
}

func TestExitCodeReportsRandomFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := exitCode(
		nil,
		&stdout,
		&stderr,
		failingReader{},
		func(string, []byte, fs.FileMode) error {
			require.FailNow(t, "write should not run after random failure")
			return nil
		},
		func(string) (int, []byte) {
			require.FailNow(t, "fetch should not run after random failure")
			return 0, nil
		},
	)

	require.Equal(t, 1, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "failed to generate JWT key: random failed")
}

func TestExitCodeReportsOpenAPIFetchFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := exitCode(
		nil,
		&stdout,
		&stderr,
		bytes.NewReader(bytes.Repeat([]byte{0x02}, 32)),
		func(string, []byte, fs.FileMode) error {
			require.FailNow(t, "write should not run after fetch failure")
			return nil
		},
		func(string) (int, []byte) {
			return http.StatusTeapot, []byte("route failed")
		},
	)

	require.Equal(t, 1, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "failed to get OpenAPI spec: status 418")
	require.Contains(t, stderr.String(), "route failed")
}

func TestExitCodeReportsWriteFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := exitCode(
		[]string{"out.json"},
		&stdout,
		&stderr,
		bytes.NewReader(bytes.Repeat([]byte{0x03}, 32)),
		func(string, []byte, fs.FileMode) error {
			return errors.New("disk full")
		},
		func(string) (int, []byte) {
			return http.StatusOK, []byte(`{"ok":true}`)
		},
	)

	require.Equal(t, 1, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "failed to write out.json: disk full")
}
