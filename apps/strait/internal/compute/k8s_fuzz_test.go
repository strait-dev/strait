package compute

import (
	"testing"
)

func FuzzK8s_ImageURI(f *testing.F) {
	// Seed corpus with known-good and known-bad values.
	f.Add("alpine:3.21")
	f.Add("ghcr.io/strait-dev/strait:v1.0.0")
	f.Add("registry.example.com:5000/image:tag")
	f.Add("")
	f.Add("../../../etc/passwd")
	f.Add("image; rm -rf /")
	f.Add("image\x00null")
	f.Add("https://evil.com/malware")
	f.Add("file:///etc/passwd")

	f.Fuzz(func(t *testing.T, uri string) {
		// validateImageURI must not panic on any input.
		_ = validateImageURI(uri)
	})
}

func FuzzK8s_MachineID(f *testing.F) {
	f.Add("strait-abc123def456")
	f.Add("")
	f.Add("../traversal")
	f.Add("\x00null\x00bytes")
	f.Add("a]b[c{d}e")

	f.Fuzz(func(t *testing.T, id string) {
		// validateMachineID must not panic on any input.
		_ = validateMachineID(id)
	})
}

func FuzzK8s_Labels(f *testing.F) {
	f.Add("key", "value")
	f.Add("", "")
	f.Add("app", "malicious-override")
	f.Add("key\x00null", "value\x00null")
	f.Add("'; DROP TABLE;--", "sqli")

	f.Fuzz(func(t *testing.T, key, value string) {
		labels := map[string]string{key: value}
		// sanitizeUserLabels must not panic on any input.
		result := sanitizeUserLabels(labels)
		// Verify no null bytes in output.
		for k, v := range result {
			for _, c := range k {
				if c < 32 || c == 127 {
					t.Errorf("sanitized key contains control char: %q", k)
				}
			}
			for _, c := range v {
				if c < 32 || c == 127 {
					t.Errorf("sanitized value contains control char: %q", v)
				}
			}
		}
	})
}

func FuzzK8s_PresetName(f *testing.F) {
	f.Add("micro")
	f.Add("small-1x")
	f.Add("medium-2x")
	f.Add("large-1x")
	f.Add("")
	f.Add("nonexistent")
	f.Add("'; DROP TABLE;--")

	f.Fuzz(func(t *testing.T, name string) {
		// PresetFromName must not panic on any input.
		_, _ = PresetFromName(name)
	})
}

func FuzzK8s_RegionCode(f *testing.F) {
	f.Add("iad")
	f.Add("lhr")
	f.Add("")
	f.Add("us-east")
	f.Add("INVALID")
	f.Add("\x00")

	f.Fuzz(func(t *testing.T, code string) {
		// Region functions must not panic on any input.
		_ = IsValidRegion(code)
		_ = NearestRegion(code)
		_ = RegionFallbackChain(code)
	})
}
