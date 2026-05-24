package worker

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignHTTPDispatch returns the HMAC-SHA256 hex signature for an HTTP
// dispatch. Format: `v1=<hex>`. Timestamp is unix seconds as decimal
// string. The signature covers `<timestamp>.<body>`.
func SignHTTPDispatch(secret string, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}
