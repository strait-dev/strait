package worker

import "testing"

func TestValidateEndpointURL_Valid(t *testing.T) {
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
	if err := validateEndpointURL("http://169.254.169.254/latest/meta-data/"); err == nil {
		t.Error("validateEndpointURL(link-local) = nil, want error")
	}
}

func TestValidateEndpointURL_InvalidScheme(t *testing.T) {
	if err := validateEndpointURL("ftp://example.com/file"); err == nil {
		t.Error("validateEndpointURL(ftp) = nil, want error for non-http(s) scheme")
	}
}

func TestValidateEndpointURL_MissingHost(t *testing.T) {
	if err := validateEndpointURL("http:///path"); err == nil {
		t.Error("validateEndpointURL(no host) = nil, want error")
	}
}

func TestValidateEndpointURL_CloudMetadata(t *testing.T) {
	// AWS metadata endpoint — must be blocked
	if err := validateEndpointURL("http://169.254.169.254/latest/meta-data/iam/security-credentials/"); err == nil {
		t.Error("validateEndpointURL(AWS metadata) = nil, want error for link-local address")
	}
}
