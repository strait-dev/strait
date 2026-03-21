package api

import (
	"net/http"
)

// requireCloudEdition returns a middleware that blocks requests when the server
// is running the community edition. Cloud-only endpoints return 402 Payment
// Required with a JSON body directing users to upgrade.
func (s *Server) requireCloudEdition(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.edition.AllowsAdvancedAnalytics() {
			next.ServeHTTP(w, r)
			return
		}
		respondJSON(w, http.StatusPaymentRequired, map[string]string{
			"error":   "this feature requires Strait Cloud",
			"edition": string(s.edition),
			"upgrade": "https://strait.dev/pricing",
		})
	})
}
