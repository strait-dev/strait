"""Tests for composition idempotency."""

from strait.composition._idempotency import with_idempotency, with_idempotency_header


class TestWithIdempotency:
    def test_adds_key_to_none_headers(self):
        result = with_idempotency(None, "key-1")
        assert result == {"Idempotency-Key": "key-1"}

    def test_adds_key_to_existing_headers(self):
        result = with_idempotency({"X-Custom": "val"}, "key-1")
        assert result == {"X-Custom": "val", "Idempotency-Key": "key-1"}

    def test_does_not_mutate_original(self):
        original = {"X-Custom": "val"}
        result = with_idempotency(original, "key-1")
        assert "Idempotency-Key" not in original
        assert "Idempotency-Key" in result

    def test_custom_header_name(self):
        result = with_idempotency_header(None, "key-1", "X-Idempotency")
        assert result == {"X-Idempotency": "key-1"}
