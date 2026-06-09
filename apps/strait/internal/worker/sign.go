package worker

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"unsafe"
)

var hmacSeparator = []byte(".")

// SignHTTPDispatch returns the HMAC-SHA256 hex signature for an HTTP
// dispatch. Format: `v1=<hex>`. Timestamp is unix seconds as decimal
// string. The signature covers `<timestamp>.<body>`.
func SignHTTPDispatch(secret string, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, unsafe.Slice(unsafe.StringData(secret), len(secret)))
	mac.Write(unsafe.Slice(unsafe.StringData(timestamp), len(timestamp)))
	mac.Write(hmacSeparator)
	mac.Write(body)

	var sum [sha256.Size]byte
	digest := mac.Sum(sum[:0])
	var out [len("v1=") + sha256.Size*2]byte
	copy(out[:], "v1=")
	hex.Encode(out[len("v1="):], digest)
	return string(out[:])
}
