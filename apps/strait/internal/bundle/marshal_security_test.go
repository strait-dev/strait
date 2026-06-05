package bundle

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalYAML_NormalSize_Passes(t *testing.T) {
	t.Parallel()

	yaml := `version: "1"
jobs:
  - name: test-job
    endpoint_url: https://example.com/webhook
`
	_, err := UnmarshalYAML([]byte(yaml))
	require.NoError(t, err)
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

	require.LessOrEqual(t, len(yaml), maxBundleYAMLSize)

	// Should not error on size (may error on content, but that's fine).
	_, err := UnmarshalYAML([]byte(yaml))
	if err != nil {
		assert.NotContains(t, err.Error(), "exceeds maximum size")
	}
}

func TestUnmarshalYAML_OverMaxSize_Rejected(t *testing.T) {
	t.Parallel()

	data := make([]byte, maxBundleYAMLSize+1)
	for i := range data {
		data[i] = 'x'
	}

	_, err := UnmarshalYAML(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum size")
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
