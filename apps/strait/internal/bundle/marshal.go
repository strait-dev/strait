package bundle

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// MarshalYAML serializes a Bundle to YAML bytes.
func MarshalYAML(b *Bundle) ([]byte, error) {
	data, err := yaml.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("marshal bundle: %w", err)
	}
	return data, nil
}

// UnmarshalYAML deserializes YAML bytes into a Bundle.
func UnmarshalYAML(data []byte) (*Bundle, error) {
	var b Bundle
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("unmarshal bundle: %w", err)
	}
	if b.Version == "" {
		return nil, fmt.Errorf("bundle version is required")
	}
	if b.Version != Version {
		return nil, fmt.Errorf("unsupported bundle version %q, expected %q", b.Version, Version)
	}
	return &b, nil
}
