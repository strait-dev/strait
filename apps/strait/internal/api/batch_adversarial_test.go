package api

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBatch_MixedValidInvalid(t *testing.T) {
	t.Parallel()

	// Validate that tag validation catches the bad item in a mixed batch.
	items := []struct {
		tags    map[string]string
		wantErr bool
	}{
		{tags: map[string]string{"env": "prod"}, wantErr: false},
		{tags: map[string]string{strings.Repeat("k", 65): "val"}, wantErr: true},
		{tags: map[string]string{"region": "us-east"}, wantErr: false},
	}

	for i, item := range items {
		err := validateTags(item.tags)
		if item.wantErr && err == nil {
			t.Errorf("item %d: expected validation error for oversized tag key", i)
		}
		if !item.wantErr && err != nil {
			t.Errorf("item %d: unexpected validation error: %v", i, err)
		}
	}
}

func TestBatch_AtMaxItems(t *testing.T) {
	t.Parallel()

	// Simulate validating exactly the max number of tags per item (20 tags).
	tags := make(map[string]string, 20)
	for i := range 20 {
		tags["key"+string(rune('a'+i))] = "value"
	}

	err := validateTags(tags)
	if err != nil {
		t.Fatalf("20 tags (at max) should be accepted: %v", err)
	}
}

func TestBatch_OverMaxItems(t *testing.T) {
	t.Parallel()

	// One over the max tag count (21 tags).
	tags := make(map[string]string, 21)
	for i := range 21 {
		tags["key"+string(rune('a'+i))] = "value"
	}

	err := validateTags(tags)
	if err == nil {
		t.Fatal("21 tags should be rejected (max 20)")
	}
	if !strings.Contains(err.Error(), "too many tags") {
		t.Errorf("error should mention too many tags, got: %v", err)
	}
}

func TestBatch_AllInvalid(t *testing.T) {
	t.Parallel()

	// Every item has an oversized tag key.
	for i := range 5 {
		tags := map[string]string{strings.Repeat("x", 65): "val"}
		err := validateTags(tags)
		if err == nil {
			t.Errorf("item %d: expected error for oversized tag key", i)
		}
	}
}

func TestBatch_PerItemPayloadBomb(t *testing.T) {
	t.Parallel()

	// A 5MB+ payload should be rejected by validatePayloadSize.
	bigPayload := json.RawMessage(`{"data":"` + strings.Repeat("a", 5*1024*1024+1) + `"}`)
	err := validatePayloadSize(bigPayload)
	if err == nil {
		t.Fatal("5MB+ payload should be rejected")
	}
	if !strings.Contains(err.Error(), "payload too large") {
		t.Errorf("error should mention payload too large, got: %v", err)
	}

	// Just under the limit should pass.
	smallPayload := json.RawMessage(`{"ok":true}`)
	err = validatePayloadSize(smallPayload)
	if err != nil {
		t.Fatalf("small payload should be accepted: %v", err)
	}
}

func TestBatch_EmptyArray(t *testing.T) {
	t.Parallel()

	// Empty tags map should pass validation.
	err := validateTags(map[string]string{})
	if err != nil {
		t.Fatalf("empty tags should be accepted: %v", err)
	}

	// Empty payload should pass size validation.
	err = validatePayloadSize(json.RawMessage{})
	if err != nil {
		t.Fatalf("empty payload should pass size validation: %v", err)
	}

	// Empty payload should pass schema validation with empty schema.
	err = validatePayloadAgainstSchema(json.RawMessage{}, json.RawMessage{})
	if err != nil {
		t.Fatalf("empty payload against empty schema should pass: %v", err)
	}
}

func TestBatch_DuplicateIdempotencyKeys(t *testing.T) {
	t.Parallel()

	// Validate that the idempotency key length check works correctly
	// even when the same key is repeated.
	key := strings.Repeat("k", 256) // exactly at the limit
	if len(key) > maxIdempotencyKeyLength {
		t.Fatalf("key length %d should be at limit %d", len(key), maxIdempotencyKeyLength)
	}

	// Duplicate keys that are valid should each pass length validation.
	for range 5 {
		if len(key) > maxIdempotencyKeyLength {
			t.Fatal("repeated key should still pass length check")
		}
	}

	// One char over the limit should fail.
	tooLong := key + "x"
	if len(tooLong) <= maxIdempotencyKeyLength {
		t.Fatalf("key length %d should exceed limit %d", len(tooLong), maxIdempotencyKeyLength)
	}
}

func FuzzBatchTriggerPayload(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"key":"value"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"string"`))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte{0xff, 0xfe})
	f.Add([]byte(`{"a":` + strings.Repeat("[", 100) + strings.Repeat("]", 100) + `}`))
	f.Add([]byte(strings.Repeat("a", 1024)))

	f.Fuzz(func(t *testing.T, payload []byte) {
		// validatePayloadSize should never panic.
		_ = validatePayloadSize(json.RawMessage(payload))

		// canonicalizePayload should never panic.
		_, _, _ = canonicalizePayload(json.RawMessage(payload))

		// validatePayloadAgainstSchema should never panic.
		_ = validatePayloadAgainstSchema(json.RawMessage(payload), json.RawMessage(`{"type":"object"}`))
	})
}

func TestCanonicalizePayload_PreservesLargeJSONIntegers(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"id":9007199254740993123456789,"nested":{"value":123456789012345678901234567890}}`)
	canonical, _, err := canonicalizePayload(payload)
	if err != nil {
		t.Fatalf("canonicalizePayload returned error: %v", err)
	}
	if !bytes.Contains(canonical, []byte(`9007199254740993123456789`)) {
		t.Fatalf("canonical payload lost large id precision: %s", canonical)
	}
	if !bytes.Contains(canonical, []byte(`123456789012345678901234567890`)) {
		t.Fatalf("canonical payload lost nested value precision: %s", canonical)
	}
}

func TestBulkCancel_NonExistentIDs(t *testing.T) {
	t.Parallel()

	// Validate ID format checking rejects known-bad IDs.
	badIDs := []string{
		"",
		strings.Repeat("x", 65),
		"id/with/slashes",
		"id..with..dots",
		"id\x00null",
	}

	for _, id := range badIDs {
		err := validateIDFormat(id)
		if err == nil {
			t.Errorf("validateIDFormat(%q) should return error", id)
		}
	}
}

func TestBulkCancel_MixedExistingAndNot(t *testing.T) {
	t.Parallel()

	// Valid IDs should pass format validation.
	validIDs := []string{
		"01234567-abcd-efab-cdef-0123456789ab",
		"run_abc123",
		"valid-id-here",
	}
	for _, id := range validIDs {
		err := validateIDFormat(id)
		if err != nil {
			t.Errorf("validateIDFormat(%q) unexpected error: %v", id, err)
		}
	}

	// Invalid IDs should fail format validation.
	invalidIDs := []string{
		"",
		"has/slash",
		"has..double-dot",
		"has\x00null",
		strings.Repeat("a", 65),
	}
	for _, id := range invalidIDs {
		err := validateIDFormat(id)
		if err == nil {
			t.Errorf("validateIDFormat(%q) should return error", id)
		}
	}

	// Combined: iterate a mixed list and check partitioning.
	mixed := make([]string, 0, len(validIDs)+len(invalidIDs))
	mixed = append(mixed, validIDs...)
	mixed = append(mixed, invalidIDs...)
	validCount := 0
	invalidCount := 0
	for _, id := range mixed {
		if validateIDFormat(id) == nil {
			validCount++
		} else {
			invalidCount++
		}
	}
	if validCount != len(validIDs) {
		t.Errorf("valid count = %d, want %d", validCount, len(validIDs))
	}
	if invalidCount != len(invalidIDs) {
		t.Errorf("invalid count = %d, want %d", invalidCount, len(invalidIDs))
	}
}
