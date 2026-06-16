package api

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

func TestDocsRequestExamplesMatchRuntimeOpenAPI(t *testing.T) {
	t.Parallel()

	spec := fetchOpenAPISpec(t)
	examplesDir := filepath.Join("..", "..", "..", "docs", "examples", "requests")
	cases := []struct {
		file   string
		method string
		path   string
	}{
		{file: "create-project.json", method: "post", path: "/v1/projects"},
		{file: "create-api-key.json", method: "post", path: "/v1/api-keys"},
		{file: "create-http-job.json", method: "post", path: "/v1/jobs"},
		{file: "create-worker-job.json", method: "post", path: "/v1/jobs"},
		{file: "trigger-run.json", method: "post", path: "/v1/jobs/{jobID}/trigger"},
		{file: "create-webhook-subscription.json", method: "post", path: "/v1/webhooks/subscriptions"},
		{file: "create-event-source.json", method: "post", path: "/v1/event-sources"},
		{file: "dispatch-event.json", method: "post", path: "/v1/events/dispatch"},
		{file: "bulk-dlq-replay.json", method: "post", path: "/v1/runs/bulk-dlq-replay"},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()

			examplePath := filepath.Join(examplesDir, tc.file)
			body, err := os.ReadFile(examplePath)
			require.NoError(t, err)

			var instance any
			require.NoError(t, json.Unmarshal(body, &instance))

			schema := compileOpenAPIRequestSchema(t, spec, tc.method, tc.path)
			require.NoError(t, schema.Validate(instance))
		})
	}
}

func compileOpenAPIRequestSchema(t *testing.T, spec map[string]any, method, path string) *jsonschema.Schema {
	t.Helper()

	ref := openAPIRequestSchemaRef(t, spec, method, path)
	schemaDoc := map[string]any{
		"$schema":    "https://json-schema.org/draft/2020-12/schema",
		"$ref":       ref,
		"components": spec["components"],
	}
	raw, err := json.Marshal(schemaDoc)
	require.NoError(t, err)

	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	require.NoError(t, compiler.AddResource("schema.json", bytes.NewReader(raw)))

	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)
	return schema
}

func openAPIRequestSchemaRef(t *testing.T, spec map[string]any, method, path string) string {
	t.Helper()

	paths, ok := spec["paths"].(map[string]any)
	require.True(t, ok)
	pathItem, ok := paths[path].(map[string]any)
	require.Truef(t, ok, "OpenAPI spec missing path %s", path)
	operation, ok := pathItem[method].(map[string]any)
	require.Truef(t, ok, "OpenAPI spec missing operation %s %s", method, path)
	requestBody, ok := operation["requestBody"].(map[string]any)
	require.Truef(t, ok, "OpenAPI operation %s %s has no request body", method, path)
	content, ok := requestBody["content"].(map[string]any)
	require.True(t, ok)
	mediaType, ok := content["application/json"].(map[string]any)
	require.True(t, ok)
	schema, ok := mediaType["schema"].(map[string]any)
	require.True(t, ok)
	ref, ok := schema["$ref"].(string)
	require.Truef(t, ok, "OpenAPI operation %s %s request schema is not a $ref", method, path)
	return ref
}
