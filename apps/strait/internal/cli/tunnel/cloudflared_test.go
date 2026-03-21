package tunnel

import (
	"strings"
	"testing"
)

func TestDetectCloudflared_NotInPath(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv.
	t.Setenv("PATH", "")

	result := DetectCloudflared()
	if result != "" {
		t.Fatalf("expected empty string when cloudflared is not in PATH, got %q", result)
	}
}

func TestDownloadURL_SupportedPlatforms(t *testing.T) {
	t.Parallel()

	// We can only test the current platform, but verify the URL is well-formed.
	url, err := DownloadURL()
	if err != nil {
		t.Fatalf("unexpected error for current platform: %v", err)
	}

	if !strings.HasPrefix(url, "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-") {
		t.Fatalf("unexpected URL format: %s", url)
	}

	if !strings.Contains(url, "darwin") && !strings.Contains(url, "linux") {
		t.Fatalf("URL should contain platform identifier: %s", url)
	}
}

func TestCachePath_ContainsCloudflared(t *testing.T) {
	t.Parallel()

	path := CachePath()
	if !strings.HasSuffix(path, "/cloudflared") && !strings.HasSuffix(path, `\cloudflared`) {
		t.Fatalf("cache path should end with cloudflared, got %q", path)
	}

	if !strings.Contains(path, ".config") {
		t.Fatalf("cache path should contain .config directory, got %q", path)
	}

	if !strings.Contains(path, "strait") {
		t.Fatalf("cache path should contain strait directory, got %q", path)
	}
}
