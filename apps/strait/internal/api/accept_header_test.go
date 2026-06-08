package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequireJSONAccept(t *testing.T) {
	t.Parallel()

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireJSONAccept(inner)

	tests := []struct {
		name   string
		accept string
		want   int
	}{
		{"application/json", "application/json", http.StatusOK},
		{"wildcard", "*/*", http.StatusOK},
		{"application wildcard", "application/*", http.StatusOK},
		{"empty accept defaults to JSON", "", http.StatusOK},
		{"json with quality", "application/json;q=0.9", http.StatusOK},
		{"csv export", "text/csv", http.StatusOK},
		{"ndjson export", "application/x-ndjson", http.StatusOK},
		{"csv export with quality", "text/csv;q=0.9", http.StatusOK},
		{"json in multi-value", "text/html, application/json", http.StatusOK},
		{"export in multi-value", "application/xml, application/x-ndjson", http.StatusOK},
		{"wildcard in multi-value", "text/html, */*", http.StatusOK},
		{"text/html only", "text/html", http.StatusNotAcceptable},
		{"application/xml only", "application/xml", http.StatusNotAcceptable},
		{"text/plain only", "text/plain", http.StatusNotAcceptable},
		{"image/png only", "image/png", http.StatusNotAcceptable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.Equal(t,
				tc.want, w.Code)
		})
	}
}

func FuzzRequireJSONAccept(f *testing.F) {
	f.Add("application/json")
	f.Add("*/*")
	f.Add("")
	f.Add("text/html")
	f.Add("application/json, text/html;q=0.9")
	f.Add(strings.Repeat("a/b, ", 100))

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireJSONAccept(inner)

	f.Fuzz(func(t *testing.T, accept string) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", accept)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.False(t,
			w.Code != http.StatusOK &&
				w.Code !=
					http.
						StatusNotAcceptable,
		)
	})
}

type acceptBenchmarkResponseWriter struct {
	header http.Header
	status int
}

func (w *acceptBenchmarkResponseWriter) Header() http.Header {
	return w.header
}

func (w *acceptBenchmarkResponseWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (w *acceptBenchmarkResponseWriter) WriteHeader(status int) {
	w.status = status
}

func BenchmarkRequireJSONAccept(b *testing.B) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := requireJSONAccept(inner)

	benchmarks := []struct {
		name   string
		accept string
	}{
		{name: "empty", accept: ""},
		{name: "json", accept: "application/json"},
		{name: "json_quality", accept: "application/json;q=0.9"},
		{name: "multi_value_late_match", accept: "text/html;q=0.8, application/xml;q=0.7, application/json;q=0.6"},
		{name: "csv_quality", accept: "text/csv;q=0.9"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if bm.accept != "" {
				req.Header.Set("Accept", bm.accept)
			}
			w := &acceptBenchmarkResponseWriter{header: make(http.Header)}

			b.ReportAllocs()
			for b.Loop() {
				w.status = 0
				handler.ServeHTTP(w, req)
				if w.status != http.StatusNoContent {
					b.Fatalf("status = %d, want %d", w.status, http.StatusNoContent)
				}
			}
		})
	}
}
