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
		assert.False(t,
			w.Code != http.StatusOK &&
				w.Code !=
					http.
						StatusUnsupportedMediaType,
		)
	})
}
