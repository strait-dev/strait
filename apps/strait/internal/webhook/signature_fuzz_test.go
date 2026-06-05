package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func FuzzValidateSignature(f *testing.F) {
	// Seed with realistic values for each algorithm path.
	body := []byte(`{"event":"test"}`)
	secret := "test-secret-value"

	// hmac-sha256 valid
	hmacSig := computeFuzzHMAC(secret, body)
	f.Add("hmac-sha256", secret, body, "sha256="+hmacSig)

	// hmac-sha256 invalid
	f.Add("hmac-sha256", secret, body, "sha256=deadbeef")
	f.Add("hmac-sha256", secret, body, "nosig")

	// github-sha256 valid
	f.Add("github-sha256", secret, body, "sha256="+hmacSig)

	// stripe-v1 valid
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	stripePayload := append([]byte(ts+"."), body...)
	stripeSig := computeFuzzHMAC(secret, stripePayload)
	f.Add("stripe-v1", secret, body, fmt.Sprintf("t=%s,v1=%s", ts, stripeSig))

	// stripe-v1 missing fields
	f.Add("stripe-v1", secret, body, "t=12345")
	f.Add("stripe-v1", secret, body, "v1=abc")
	f.Add("stripe-v1", secret, body, "t=notanumber,v1=abc")

	// unsupported algorithm
	f.Add("unknown", secret, body, "anything")
	f.Add("", "", []byte{}, "")

	f.Fuzz(func(t *testing.T, algorithm, secret string, body []byte, headerValue string) {
		// Must not panic on any input. Errors are expected.
		_ = ValidateSignature(algorithm, secret, body, headerValue)
	})
}

func FuzzValidateHMACSHA256(f *testing.F) {
	body := []byte(`{"event":"test"}`)
	secret := "test-secret-value"
	sig := computeFuzzHMAC(secret, body)

	f.Add(secret, body, "sha256="+sig)
	f.Add(secret, body, "sha256=0000000000000000000000000000000000000000000000000000000000000000")
	f.Add(secret, body, "deadbeef")
	f.Add(secret, body, "sha256=")
	f.Add("", []byte{}, "sha256="+computeFuzzHMAC("", []byte{}))
	f.Add("key", []byte("large payload with special chars: \x00\xff\n\t"), "sha256=abc")

	f.Fuzz(func(t *testing.T, secret string, body []byte, headerValue string) {
		// Must not panic on any input. Errors are expected.
		_ = validateHMACSHA256(secret, body, headerValue)
	})
}

func FuzzValidateStripeV1(f *testing.F) {
	secret := "whsec_test123"
	body := []byte(`{"id":"evt_1"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	payload := append([]byte(ts+"."), body...)
	sig := computeFuzzHMAC(secret, payload)

	f.Add(secret, body, fmt.Sprintf("t=%s,v1=%s", ts, sig))
	f.Add(secret, body, fmt.Sprintf("t=%s,v1=deadbeef", ts))
	f.Add(secret, body, "t=0,v1=abc")
	f.Add(secret, body, "t=9999999999999,v1=abc")
	f.Add(secret, body, "t=notanumber,v1=abc")
	f.Add(secret, body, "v1=abc")
	f.Add(secret, body, "t=12345")
	f.Add(secret, body, "")
	f.Add(secret, body, "garbage")
	f.Add(secret, body, "t=,v1=")
	f.Add(secret, body, "t=123,v1=abc,v0=extra,foo=bar")

	f.Fuzz(func(t *testing.T, secret string, body []byte, headerValue string) {
		// Must not panic on any input. Errors are expected.
		_ = validateStripeV1(secret, body, headerValue)
	})
}

func computeFuzzHMAC(secret string, data []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

// FuzzComputeHMACSHA256 verifies that ComputeHMACSHA256 never panics
// and always returns a 64-character hex string.
func FuzzComputeHMACSHA256(f *testing.F) {
	f.Add("secret", []byte("body"))
	f.Add("", []byte{})
	f.Add("key", []byte("\x00\xff\n\t"))
	f.Add(string(make([]byte, 256)), []byte("short"))
	f.Add("key", make([]byte, 1<<16))

	f.Fuzz(func(t *testing.T, secret string, body []byte) {
		result := ComputeHMACSHA256(secret, body)
		assert.Len(t, result,
			64)

	})
}
