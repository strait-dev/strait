package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateEndpointURL_Valid(t *testing.T) {
	t.Parallel()
	urls := []string{
		"https://example.com/webhook",
		"http://api.example.com:8080/path",
		"https://93.184.216.34/endpoint",
	}
	for _, u := range urls {
		assert.NoError(t, validateEndpointURL(u))

	}
}

func TestValidateEndpointURL_PrivateIP(t *testing.T) {
	t.Parallel()
	urls := []string{
		"http://10.0.0.1/admin",
		"http://192.168.1.1/internal",
		"http://172.16.0.1/secret",
	}
	for _, u := range urls {
		assert.Error(
			t,
			validateEndpointURL(u))

	}
}

func TestValidateEndpointURL_Loopback(t *testing.T) {
	t.Parallel()
	urls := []string{
		"http://127.0.0.1/metadata",
		"http://127.0.0.1:9000/admin",
	}
	for _, u := range urls {
		assert.Error(
			t,
			validateEndpointURL(u))

	}
}

func TestValidateEndpointURL_LinkLocal(t *testing.T) {
	t.Parallel()
	assert.Error(
		t,
		validateEndpointURL("http://169.254.169.254/latest/meta-data/"),
	)

}

func TestValidateEndpointURL_InvalidScheme(t *testing.T) {
	t.Parallel()
	assert.Error(
		t,
		validateEndpointURL("ftp://example.com/file"))

}

func TestValidateEndpointURL_MissingHost(t *testing.T) {
	t.Parallel()
	assert.Error(
		t,
		validateEndpointURL("http:///path"))

}

func TestValidateEndpointURL_CloudMetadata(t *testing.T) {
	t.Parallel()
	assert.Error(
		t,
		validateEndpointURL("http://169.254.169.254/latest/meta-data/iam/security-credentials/"))

}

func TestValidateEndpointURL_CGNAT(t *testing.T) {
	t.Parallel()
	urls := []string{
		"http://100.64.0.1/internal",
		"http://100.100.100.100/admin",
		"http://100.127.255.254/secret",
	}
	for _, u := range urls {
		assert.Error(
			t,
			validateEndpointURL(u))

	}
}

func TestValidateEndpointURL_IPv6ULA(t *testing.T) {
	t.Parallel()
	urls := []string{
		"http://[fc00::1]/internal",
		"http://[fd12:3456:789a::1]/admin",
		"http://[fdff:ffff:ffff::1]/secret",
	}
	for _, u := range urls {
		assert.Error(
			t,
			validateEndpointURL(u))

	}
}

func TestValidateEndpointURL_CGNATBoundary(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validateEndpointURL("http://100.63.255.255/ok"))
	assert.NoError(t, validateEndpointURL("http://100.128.0.0/ok"))

}

func FuzzValidateEndpointURL(f *testing.F) {
	f.Add("https://example.com/webhook")
	f.Add("http://127.0.0.1/admin")
	f.Add("ftp://evil.com")
	f.Add("")
	f.Add("http://169.254.169.254/latest")
	f.Add("not-a-url")
	f.Add("http://example.com:8080/path")
	f.Add("http://localhost/secret")
	f.Add("http://[::1]/path")
	f.Add("http://10.0.0.1/internal")
	f.Add("http://192.168.1.1/private")
	f.Add("http://100.64.0.1/cgnat")
	f.Add("https://example.com:99999")
	f.Add("http://metadata.google.internal/computeMetadata/v1/")

	f.Fuzz(func(t *testing.T, rawURL string) {
		// ValidateEndpointURL should never panic regardless of input.
		_ = ValidateEndpointURL(rawURL)
	})
}
