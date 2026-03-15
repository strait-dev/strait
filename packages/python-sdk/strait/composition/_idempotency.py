"""Idempotency key header injection."""

from __future__ import annotations


def with_idempotency(headers: dict[str, str] | None, key: str) -> dict[str, str]:
    return with_idempotency_header(headers, key, "Idempotency-Key")


def with_idempotency_header(
    headers: dict[str, str] | None, key: str, header_name: str,
) -> dict[str, str]:
    result = dict(headers) if headers else {}
    result[header_name] = key
    return result
