package worker

import "testing"

func TestValidateEndpointURL_Valid(t *testing.T) {
	t.Parallel()
	urls := []string{
		"https://example.com/webhook",
		"http://api.example.com:8080/path",
		"https://93.184.216.34/endpoint",
	}
	for _, u := range urls {
		if err := validateEndpointURL(u); err != nil {
			t.Errorf("validateEndpointURL(%q) = %v, want nil", u, err)
		}
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
		if err := validateEndpointURL(u); err == nil {
			t.Errorf("validateEndpointURL(%q) = nil, want error for private IP", u)
		}
	}
}

func TestValidateEndpointURL_Loopback(t *testing.T) {
	t.Parallel()
	urls := []string{
		"http://127.0.0.1/metadata",
		"http://127.0.0.1:9000/admin",
	}
	for _, u := range urls {
		if err := validateEndpointURL(u); err == nil {
			t.Errorf("validateEndpointURL(%q) = nil, want error for loopback", u)
		}
	}
}

func TestValidateEndpointURL_LinkLocal(t *testing.T) {
	t.Parallel()
	if err := validateEndpointURL("http://169.254.169.254/latest/meta-data/"); err == nil {
		t.Error("validateEndpointURL(link-local) = nil, want error")
	}
}

func TestValidateEndpointURL_InvalidScheme(t *testing.T) {
	t.Parallel()
	if err := validateEndpointURL("ftp://example.com/file"); err == nil {
		t.Error("validateEndpointURL(ftp) = nil, want error for non-http(s) scheme")
	}
}

func TestValidateEndpointURL_MissingHost(t *testing.T) {
	t.Parallel()
	if err := validateEndpointURL("http:///path"); err == nil {
		t.Error("validateEndpointURL(no host) = nil, want error")
	}
}

func TestValidateEndpointURL_CloudMetadata(t *testing.T) {
	t.Parallel()
	if err := validateEndpointURL("http://169.254.169.254/latest/meta-data/iam/security-credentials/"); err == nil {
		t.Error("validateEndpointURL(AWS metadata) = nil, want error for link-local address")
	}
}

func TestValidateEndpointURL_CGNAT(t *testing.T) {
	t.Parallel()
	urls := []string{
		"http://100.64.0.1/internal",
		"http://100.100.100.100/admin",
		"http://100.127.255.254/secret",
	}
	for _, u := range urls {
		if err := validateEndpointURL(u); err == nil {
			t.Errorf("validateEndpointURL(%q) = nil, want error for CGNAT address", u)
		}
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
		if err := validateEndpointURL(u); err == nil {
			t.Errorf("validateEndpointURL(%q) = nil, want error for IPv6 ULA address", u)
		}
	}
}

func TestValidateEndpointURL_CGNATBoundary(t *testing.T) {
	t.Parallel()
	if err := validateEndpointURL("http://100.63.255.255/ok"); err != nil {
		t.Errorf("validateEndpointURL(just below CGNAT) = %v, want nil", err)
	}
	if err := validateEndpointURL("http://100.128.0.0/ok"); err != nil {
		t.Errorf("validateEndpointURL(just above CGNAT) = %v, want nil", err)
	}
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
