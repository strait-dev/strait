package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequireJSONContentType(t *testing.T) {
	t.Parallel()

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireJSONContentType(inner)

	tests := []struct {
		name        string
		method      string
		contentType string
		body        string
		want        int
	}{
		{"POST with application/json", http.MethodPost, "application/json", `{"a":1}`, http.StatusOK},
		{"POST with charset", http.MethodPost, "application/json; charset=utf-8", `{"a":1}`, http.StatusOK},
		{"POST with text/xml", http.MethodPost, "text/xml", "<x/>", http.StatusUnsupportedMediaType},
		{"POST with form-urlencoded", http.MethodPost, "application/x-www-form-urlencoded", "a=1", http.StatusUnsupportedMediaType},
		{"POST with empty content-type and body", http.MethodPost, "", "some body", http.StatusUnsupportedMediaType},
		{"PUT with application/json", http.MethodPut, "application/json", `{"a":1}`, http.StatusOK},
		{"PATCH with application/json", http.MethodPatch, "application/json", `{"a":1}`, http.StatusOK},
		{"GET with any content-type", http.MethodGet, "text/html", "", http.StatusOK},
		{"DELETE with no body", http.MethodDelete, "", "", http.StatusOK},
		{"POST with no content-type and no body", http.MethodPost, "", "", http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, "/", strings.NewReader(tc.body))
			} else {
				req = httptest.NewRequest(tc.method, "/", nil)
			}
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.Equal(t,
				tc.want, w.Code)
		})
	}
}

func FuzzRequireJSONContentType(f *testing.F) {
	f.Add("application/json")
	f.Add("text/xml")
	f.Add("")
	f.Add("application/json; charset=utf-8")
	f.Add("multipart/form-data; boundary=something")

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireJSONContentType(inner)

	f.Fuzz(func(t *testing.T, ct string) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body"))
		req.Header.Set("Content-Type", ct)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Contains(t, []int{http.StatusOK, http.StatusUnsupportedMediaType}, w.Code)
	})
}

type contentTypeBenchmarkResponseWriter struct {
	header http.Header
	status int
}

func (w *contentTypeBenchmarkResponseWriter) Header() http.Header {
	return w.header
}

func (w *contentTypeBenchmarkResponseWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (w *contentTypeBenchmarkResponseWriter) WriteHeader(status int) {
	w.status = status
}

func BenchmarkRequireJSONContentType(b *testing.B) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := requireJSONContentType(inner)

	benchmarks := []struct {
		name        string
		method      string
		contentType string
		body        string
		want        int
	}{
		{name: "get_bypass", method: http.MethodGet, contentType: "text/html", want: http.StatusNoContent},
		{name: "json", method: http.MethodPost, contentType: "application/json", body: "{}", want: http.StatusNoContent},
		{name: "json_charset", method: http.MethodPost, contentType: "application/json; charset=utf-8", body: "{}", want: http.StatusNoContent},
		{name: "xml_rejected", method: http.MethodPost, contentType: "text/xml", body: "<x/>", want: http.StatusUnsupportedMediaType},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			var req *http.Request
			if bm.body != "" {
				req = httptest.NewRequest(bm.method, "/", strings.NewReader(bm.body))
			} else {
				req = httptest.NewRequest(bm.method, "/", nil)
			}
			if bm.contentType != "" {
				req.Header.Set("Content-Type", bm.contentType)
			}
			w := &contentTypeBenchmarkResponseWriter{header: make(http.Header)}

			b.ReportAllocs()
			for b.Loop() {
				w.status = 0
				handler.ServeHTTP(w, req)
				if w.status != bm.want {
					b.Fatalf("status = %d, want %d", w.status, bm.want)
				}
			}
		})
	}
}
