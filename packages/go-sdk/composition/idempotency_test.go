package composition

import "testing"

func TestWithIdempotency(t *testing.T) {
	headers := WithIdempotency(nil, "key_123")
	if headers["Idempotency-Key"] != "key_123" {
		t.Errorf("expected 'key_123', got %q", headers["Idempotency-Key"])
	}
}

func TestWithIdempotency_PreservesExisting(t *testing.T) {
	existing := map[string]string{"X-Custom": "val"}
	headers := WithIdempotency(existing, "key_456")

	if headers["X-Custom"] != "val" {
		t.Error("expected existing header preserved")
	}
	if headers["Idempotency-Key"] != "key_456" {
		t.Error("expected idempotency key added")
	}
	// original should not be modified
	if _, ok := existing["Idempotency-Key"]; ok {
		t.Error("original map should not be modified")
	}
}

func TestWithIdempotencyHeader_CustomName(t *testing.T) {
	headers := WithIdempotencyHeader(nil, "key_789", "X-Idempotent")
	if headers["X-Idempotent"] != "key_789" {
		t.Errorf("expected custom header, got %v", headers)
	}
}
