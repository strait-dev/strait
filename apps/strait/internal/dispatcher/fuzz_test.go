package dispatcher

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// FuzzQueueDepth_ResponseBody verifies that no response body can make
// queueDepth() panic, and that the return value is always a valid non-negative
// int64 or math.MaxInt64 (the error sentinel).
func FuzzQueueDepth_ResponseBody(f *testing.F) {
	// Seed with known Prometheus response shapes.
	f.Add([]byte(`{"data":{"result":[{"value":[0,"42"]}]}}`))
	f.Add([]byte(`{"data":{"result":[]}}`))
	f.Add([]byte(`{"data":{"result":[{"value":[0,"0"]}]}}`))
	f.Add([]byte(`{"data":{"result":[{"value":[0,"+Inf"]}]}}`))
	f.Add([]byte(`{"data":{"result":[{"value":[0,"NaN"]}]}}`))
	f.Add([]byte(`{"data":{"result":[{"value":[0,"-1"]}]}}`))
	f.Add([]byte(`{"data":{"result":[{"value":[0,"42.7"]}]}}`))
	f.Add([]byte(`not json`))
	f.Add([]byte(``))
	f.Add([]byte(`null`))
	f.Add([]byte(`{}`))

	f.Fuzz(func(t *testing.T, body []byte) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(body)
		}))
		defer srv.Close()

		got := queueDepth(
			context.Background(),
			srv.URL,
			&http.Client{Timeout: 2 * time.Second},
		)

		// Must never panic; return value must be >= 0 or exactly MaxInt64.
		if got < 0 && got != math.MaxInt64 {
			t.Errorf("queueDepth() = %d, want >= 0 or MaxInt64", got)
		}
	})
}

// FuzzValidateClusterURL verifies that no string input can make validateClusterURL panic.
func FuzzValidateClusterURL(f *testing.F) {
	f.Add("https://api.strait.dev")
	f.Add("http://api.strait.dev")
	f.Add("")
	f.Add("https://")
	f.Add("file:///etc/passwd")
	f.Add("javascript:alert(1)")
	f.Add("https://user:pass@host.com")
	f.Add("https://169.254.169.254/latest/meta-data/")
	f.Add("://broken")
	f.Add("\x00\xff\n")
	f.Add("https://[invalid")
	f.Add("https://[::1]")

	f.Fuzz(func(t *testing.T, rawURL string) {
		// Must never panic regardless of input.
		_ = validateClusterURL("api_url", "test-cluster", rawURL, true)
		_ = validateClusterURL("api_url", "test-cluster", rawURL, false)
	})
}

// FuzzValidateEntries verifies that no combination of cluster names and URLs
// can make validateEntries panic.
func FuzzValidateEntries(f *testing.F) {
	f.Add("honolulu", "https://api.strait.dev", "tahoe", "https://api2.strait.dev")
	f.Add("", "https://api.strait.dev", "tahoe", "https://api2.strait.dev")
	f.Add("dup", "https://api.strait.dev", "dup", "https://api2.strait.dev")
	f.Add("a", "http://api.strait.dev", "b", "https://api2.strait.dev")
	f.Add("a", "", "b", "https://api2.strait.dev")

	f.Fuzz(func(t *testing.T, name1, url1, name2, url2 string) {
		entries := []ClusterEntry{
			{Name: name1, APIURL: url1},
			{Name: name2, APIURL: url2},
		}
		// Must never panic.
		_ = validateEntries(entries)
	})
}
