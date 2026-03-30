package bundle

import (
	"strings"
	"testing"
)

func TestUnmarshalYAML_NormalSize_Passes(t *testing.T) {
	t.Parallel()

	yaml := `version: "1"
jobs:
  - name: test-job
    endpoint_url: https://example.com/webhook
`
	_, err := UnmarshalYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmarshalYAML_AtMaxSize_Passes(t *testing.T) {
	t.Parallel()

	// Create YAML just at the 1MB boundary.
	header := `version: "1"
jobs:
  - name: test-job
    endpoint_url: https://example.com/webhook
    description: "`
	footer := `"
`
	padding := strings.Repeat("x", maxBundleYAMLSize-len(header)-len(footer))
	yaml := header + padding + footer

	if len(yaml) > maxBundleYAMLSize {
		t.Fatalf("test setup: YAML size %d > max %d", len(yaml), maxBundleYAMLSize)
	}

	// Should not error on size (may error on content, but that's fine).
	_, err := UnmarshalYAML([]byte(yaml))
	if err != nil && strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("should not reject YAML at max size boundary: %v", err)
	}
}

func TestUnmarshalYAML_OverMaxSize_Rejected(t *testing.T) {
	t.Parallel()

	data := make([]byte, maxBundleYAMLSize+1)
	for i := range data {
		data[i] = 'x'
	}

	_, err := UnmarshalYAML(data)
	if err == nil {
		t.Fatal("expected error for oversized YAML")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("error = %q, want contains 'exceeds maximum size'", err.Error())
	}
}

func FuzzUnmarshalYAML(f *testing.F) {
	f.Add([]byte(`version: "1"`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))
	f.Add([]byte(`invalid yaml: [`))
	f.Add([]byte(`version: "99"`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Should never panic regardless of input.
		_, _ = UnmarshalYAML(data)
	})
}
