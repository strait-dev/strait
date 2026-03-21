package compute

import (
	"testing"
)

func TestValidateImageURI_Valid(t *testing.T) {
	valid := []string{
		"nginx",
		"nginx:latest",
		"my-registry.com/my-image:v1.2.3",
		"ghcr.io/owner/repo:sha-abc123",
		"registry.example.com:5000/path/image:tag",
		"image@sha256:abcdef1234567890",
		"my_image",
		"UPPERCASE/Image:Tag",
	}
	for _, uri := range valid {
		if err := validateImageURI(uri); err != nil {
			t.Errorf("validateImageURI(%q) = %v, want nil", uri, err)
		}
	}
}

func TestValidateImageURI_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"image;rm -rf /",
		"image|cat /etc/passwd",
		"$(whoami)",
		"`whoami`",
		"image name with spaces",
		"image\nnewline",
		"image&bg",
		"image>redirect",
		"image<input",
	}
	for _, uri := range invalid {
		if err := validateImageURI(uri); err == nil {
			t.Errorf("validateImageURI(%q) = nil, want error", uri)
		}
	}
}

func TestValidateEnvKey_Valid(t *testing.T) {
	valid := []string{
		"MY_VAR",
		"MY_VAR_123",
		"HOME",
		"a",
		"A",
		"_LEADING",
		"lowercase",
	}
	for _, key := range valid {
		if err := validateEnvKey(key); err != nil {
			t.Errorf("validateEnvKey(%q) = %v, want nil", key, err)
		}
	}
}

func TestValidateEnvKey_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"KEY=VALUE",
		"KEY;whoami",
		"KEY WITH SPACES",
		"KEY\tTAB",
		"KEY.DOT",
		"KEY-DASH",
		"KEY$VAR",
	}
	for _, key := range invalid {
		if err := validateEnvKey(key); err == nil {
			t.Errorf("validateEnvKey(%q) = nil, want error", key)
		}
	}
}

func TestValidateLabelKey_Valid(t *testing.T) {
	valid := []string{
		"com.example.label",
		"my-label",
		"my_label",
		"label123",
		"com.docker.compose.service",
	}
	for _, key := range valid {
		if err := validateLabelKey(key); err != nil {
			t.Errorf("validateLabelKey(%q) = %v, want nil", key, err)
		}
	}
}

func TestValidateLabelKey_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"label;injection",
		"label with spaces",
		"label=value",
		"label$var",
		"label`cmd`",
	}
	for _, key := range invalid {
		if err := validateLabelKey(key); err == nil {
			t.Errorf("validateLabelKey(%q) = nil, want error", key)
		}
	}
}
