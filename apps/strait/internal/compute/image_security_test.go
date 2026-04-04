package compute

import (
	"strings"
	"testing"
)

// TestImageURI_RegistryInjection documents that arbitrary registries are accepted
// by validateImageURI. The function only validates character safety, not registry
// allow-lists, so any registry hostname passes validation.
func TestImageURI_RegistryInjection(t *testing.T) {
	t.Parallel()

	// Arbitrary registry hostnames are accepted because validateImageURI only
	// checks for shell-safe characters, not an allow-list of registries.
	uri := "evil.com/nginx:latest"
	err := validateImageURI(uri)
	if err != nil {
		t.Fatalf("expected evil.com registry to be accepted (no allow-list), got error: %v", err)
	}
}

// TestImageURI_DigestBypass verifies that both tag-based and digest-based image
// references are accepted. There is no enforcement requiring digest pinning.
func TestImageURI_DigestBypass(t *testing.T) {
	t.Parallel()

	tagged := "nginx:latest"
	if err := validateImageURI(tagged); err != nil {
		t.Fatalf("expected tagged URI to be accepted, got: %v", err)
	}

	digested := "nginx@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	if err := validateImageURI(digested); err != nil {
		t.Fatalf("expected digest URI to be accepted, got: %v", err)
	}
}

// TestImageURI_TagMutation verifies that mutable tags like :latest are accepted.
// No digest pinning is enforced by validateImageURI.
func TestImageURI_TagMutation(t *testing.T) {
	t.Parallel()

	uri := "nginx:latest"
	if err := validateImageURI(uri); err != nil {
		t.Fatalf("expected :latest tag to be accepted (no digest pinning), got: %v", err)
	}
}

// TestImageURI_PrivateRegistryCredentials verifies that credentials embedded in
// a URI via user:pass@host are rejected to prevent credential leakage.
func TestImageURI_PrivateRegistryCredentials(t *testing.T) {
	t.Parallel()

	uri := "user:pass@registry.com/image:tag"
	err := validateImageURI(uri)
	if err == nil {
		t.Fatal("expected credential URI to be rejected")
	}
	if !strings.Contains(err.Error(), "embedded credentials") {
		t.Fatalf("expected embedded credentials error, got: %v", err)
	}
}

// TestImageURI_LocalFileImage verifies that file:// scheme URIs are rejected
// because they contain characters not in the safe set.
func TestImageURI_LocalFileImage(t *testing.T) {
	t.Parallel()

	// file:///etc/shadow -- while /:. are safe, the empty result of
	// file:///etc/shadow actually only has safe chars. Let us verify.
	// All chars in "file:///etc/shadow" are in [a-z/:.] so this would pass.
	// However the real concern is non-image URIs. We document the behavior.
	uri := "file:///etc/shadow"
	err := validateImageURI(uri)

	// URL scheme (file://) is now rejected by validateImageURI.
	if err == nil {
		t.Fatal("expected file:// URI to be rejected (contains URL scheme)")
	}
}

// TestImageURI_DataURIScheme verifies that data: URIs with base64 content are
// rejected because they contain characters outside the safe set.
func TestImageURI_DataURIScheme(t *testing.T) {
	t.Parallel()

	uri := "data:application/octet-stream;base64,SGVsbG8="
	err := validateImageURI(uri)
	if err == nil {
		t.Fatal("expected data URI to be rejected (contains ; and , and =)")
	}
}

// TestImageURI_ExtremelyLongURI verifies that validateImageURI processes very
// long URIs without crashing. There is no length limit enforced.
func TestImageURI_ExtremelyLongURI(t *testing.T) {
	t.Parallel()

	// 100KB URI composed of safe characters — now rejected by 255-char limit.
	uri := "registry.example.com/" + strings.Repeat("a", 100*1024) + ":latest"
	err := validateImageURI(uri)

	if err == nil {
		t.Fatal("expected extremely long URI to be rejected (255 char limit)")
	}
}

// TestImageURI_UnicodeHomoglyph verifies that Cyrillic lookalike characters are
// rejected because they fall outside the ASCII safe character set.
func TestImageURI_UnicodeHomoglyph(t *testing.T) {
	t.Parallel()

	// The 'o' in 'docker' is replaced with Cyrillic small letter 'o' (U+043E).
	uri := "d\u043Ecker.io/nginx:latest"
	err := validateImageURI(uri)
	if err == nil {
		t.Fatal("expected Cyrillic homoglyph to be rejected")
	}
}

// TestImageURI_NewlineInTag verifies that newline characters in image tags are
// rejected, preventing header/command injection via image references.
func TestImageURI_NewlineInTag(t *testing.T) {
	t.Parallel()

	uri := "nginx:latest\nRUN malicious"
	err := validateImageURI(uri)
	if err == nil {
		t.Fatal("expected newline in tag to be rejected")
	}
}

// TestImageURI_WildcardTag checks whether wildcard characters in tags are
// accepted or rejected by the validator.
func TestImageURI_WildcardTag(t *testing.T) {
	t.Parallel()

	uri := "nginx:*"
	err := validateImageURI(uri)
	if err == nil {
		t.Fatal("expected wildcard * in tag to be rejected")
	}
}

// FuzzImageURI sends random byte strings through validateImageURI to ensure it
// never panics regardless of input.
func FuzzImageURI(f *testing.F) {
	f.Add("nginx:latest")
	f.Add("")
	f.Add("evil.com/image:tag")
	f.Add("image;rm -rf /")
	f.Add("data:application/octet-stream;base64,SGVsbG8=")
	f.Add("d\u043Ecker.io/nginx:latest")
	f.Add(strings.Repeat("a", 10000))

	f.Fuzz(func(t *testing.T, uri string) {
		// Must not panic regardless of input.
		_ = validateImageURI(uri)
	})
}

// TestImageURI_EmptyRegistry verifies behavior with URIs that have empty
// registry components.
func TestImageURI_EmptyRegistry(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		uri  string
	}{
		{name: "slash colon", uri: "/:latest"},
		{name: "slash image", uri: "/nginx:latest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// These contain only safe characters, so the char validator accepts
			// them. Docker would reject them as malformed references.
			err := validateImageURI(tc.uri)
			if err != nil {
				t.Fatalf("expected %q to be accepted by char validator, got: %v", tc.uri, err)
			}
		})
	}
}

// TestImageURI_DoubleSlashBypass verifies that double-slash prefixed URIs are
// handled by the validator.
func TestImageURI_DoubleSlashBypass(t *testing.T) {
	t.Parallel()

	uri := "//evil.com/nginx:latest"
	err := validateImageURI(uri)

	// All characters are in the safe set, so this passes character validation.
	// The runtime (Docker/Fly) would reject it as a malformed reference.
	if err != nil {
		t.Fatalf("expected double-slash URI to pass char validation, got: %v", err)
	}
}

// TestImageURI_PortInjection verifies that URIs with out-of-range port numbers
// are accepted by the character validator, which does not parse port semantics.
func TestImageURI_PortInjection(t *testing.T) {
	t.Parallel()

	uri := "registry.com:99999/image:tag"
	err := validateImageURI(uri)

	// The validator checks characters only, not port number ranges.
	if err != nil {
		t.Fatalf("expected out-of-range port to be accepted by char validator, got: %v", err)
	}
}
