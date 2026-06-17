package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

type failingAuditSchemaWriter struct{}

func (failingAuditSchemaWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestRunWritesAuditSchema(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	require.NoError(t, run(&stdout, &stderr))
	require.Empty(t, stderr.String())

	var doc schemaDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Equal(t, "https://json-schema.org/draft/2020-12/schema", doc.Schema)
	require.Equal(t, "Strait audit event registry", doc.Title)
	require.Equal(t, "object", doc.Type)
	require.Equal(t, domain.AuditEventSchemaVersionCurrent, mustParseSchemaVersion(t, doc.Version))
	for _, action := range domain.KnownAuditActions() {
		_, ok := doc.Defs[action]
		require.Truef(t, ok, "missing schema definition for %s", action)
	}
}

func TestRunReturnsEncodeError(t *testing.T) {
	var stderr bytes.Buffer

	err := run(failingAuditSchemaWriter{}, &stderr)
	require.Error(t, err)
	require.Contains(t, stderr.String(), "encode: write failed")
}

func TestExitCodeReflectsRunResult(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	require.Zero(t, exitCode(&stdout, &stderr))

	stderr.Reset()
	require.Equal(t, 1, exitCode(failingAuditSchemaWriter{}, &stderr))
	require.Contains(t, stderr.String(), "encode: write failed")
}

func mustParseSchemaVersion(t *testing.T, version string) uint16 {
	t.Helper()

	var got uint16
	require.NoError(t, json.Unmarshal([]byte(version), &got))
	return got
}
