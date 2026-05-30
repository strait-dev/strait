package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// signedStripeHeader builds a valid stripe-v1 header (t=<ts>,v1=<hmac>) for the
// given body so the benchmark exercises the success path (full HMAC compare).
func signedStripeHeader(secret string, body []byte) string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return fmt.Sprintf("t=%s,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

// BenchmarkValidateStripeV1 measures signature verification across body sizes.
// validateStripeV1 runs on the inbound event-source webhook ingestion path
// (api/event_sources.go), once per delivered event.
func BenchmarkValidateStripeV1(b *testing.B) {
	const secret = "whsec_benchmark_secret_value"
	sizes := []int{256, 4 * 1024, 64 * 1024}

	for _, size := range sizes {
		body := []byte(`{"id":"evt_1","data":"` + strings.Repeat("x", size) + `"}`)
		header := signedStripeHeader(secret, body)
		b.Run(fmt.Sprintf("body_%dB", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := validateStripeV1(secret, body, header); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
