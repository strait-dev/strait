package main

import (
	"strings"
	"testing"
)

func TestDecodeManifestReader(t *testing.T) {
	t.Parallel()

	input := `
apiVersion: v1
kind: Job
metadata:
  name: send-email
spec:
  project_id: proj_1
  endpoint_url: https://example.com/hook
---
kind: Workflow
metadata:
  name: daily-workflow
spec:
  project_id: proj_1
`

	manifests, err := decodeManifestReader("test", strings.NewReader(input))
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(manifests) != 2 {
		t.Fatalf("expected 2 manifests, got %d", len(manifests))
	}
}

func TestValidateManifest(t *testing.T) {
	t.Parallel()

	err := validateManifest(manifest{
		Kind:     "Job",
		Metadata: manifestMeta{Name: "send-email"},
		Spec: map[string]any{
			"project_id":   "proj_1",
			"endpoint_url": "https://example.com",
		},
	})
	if err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
}
