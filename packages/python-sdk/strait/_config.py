"""Strait SDK configuration."""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from enum import StrEnum

from strait._errors import ValidationError


class AuthType(StrEnum):
    BEARER = "bearer"
    API_KEY = "apiKey"
    RUN_TOKEN = "runToken"


@dataclass(frozen=True)
class AuthMode:
    type: AuthType
    token: str


@dataclass
class Config:
    base_url: str
    auth: AuthMode
    default_headers: dict[str, str] = field(default_factory=dict)
    timeout_ms: int = 30_000


def normalize_base_url(url: str) -> str:
    """Strip trailing slashes from a base URL."""
    return url.rstrip("/")


def get_authorization_header(auth: AuthMode) -> str:
    """Return the Authorization header value for the auth mode."""
    return f"Bearer {auth.token}"


def config_from_env() -> Config:
    """Read configuration from environment variables.

    Environment variables:
      - STRAIT_BASE_URL (required)
      - STRAIT_API_KEY (required)
      - STRAIT_AUTH_TYPE (optional, defaults to "apiKey")
      - STRAIT_TIMEOUT_MS (optional, defaults to 30000)
    """
    base_url = os.environ.get("STRAIT_BASE_URL", "")
    if not base_url:
        raise ValidationError(
            "STRAIT_BASE_URL environment variable is required",
            issues=["STRAIT_BASE_URL is not set"],
        )

    api_key = os.environ.get("STRAIT_API_KEY", "")
    if not api_key:
        raise ValidationError(
            "STRAIT_API_KEY environment variable is required",
            issues=["STRAIT_API_KEY is not set"],
        )

    auth_type_str = os.environ.get("STRAIT_AUTH_TYPE", "")
    auth_type = AuthType(auth_type_str) if auth_type_str else AuthType.API_KEY

    timeout_ms = 30_000
    timeout_str = os.environ.get("STRAIT_TIMEOUT_MS", "")
    if timeout_str:
        try:
            timeout_ms = int(timeout_str)
        except ValueError:
            raise ValidationError(
                f'STRAIT_TIMEOUT_MS must be an integer, got "{timeout_str}"',
                issues=["STRAIT_TIMEOUT_MS is not a valid integer"],
            )

    return Config(
        base_url=normalize_base_url(base_url),
        auth=AuthMode(type=auth_type, token=api_key),
        timeout_ms=timeout_ms,
    )
