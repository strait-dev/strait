package composition

// WithIdempotency returns a new header map with the idempotency key added.
// If headers is nil, a new map is created.
func WithIdempotency(headers map[string]string, key string) map[string]string {
	return WithIdempotencyHeader(headers, key, "Idempotency-Key")
}

// WithIdempotencyHeader returns a new header map with the idempotency key
// using a custom header name.
func WithIdempotencyHeader(headers map[string]string, key string, headerName string) map[string]string {
	result := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		result[k] = v
	}
	result[headerName] = key
	return result
}
