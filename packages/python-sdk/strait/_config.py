"""Strait SDK configuration."""

from __future__ import annotations

import json
import os
from dataclasses import dataclass, field
from enum import StrEnum
from pathlib import Path

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


def _resolve_config_file_path(
    path: str | None = None, search_dir: str | None = None
) -> str:
    if path is not None:
        return path
    directory = search_dir or "."
    return str(Path(directory) / "strait.json")


def config_from_file(
    path: str | None = None, search_dir: str | None = None
) -> Config:
    """Read SDK configuration from a strait.json file.

    Reads the ``sdk`` section from the JSON file, then layers environment
    variable overrides on top (env vars always win).

    Args:
        path: Explicit path to a config file. Overrides search_dir.
        search_dir: Directory to look for ``strait.json`` in. Defaults to ``"."``.
    """
    file_path = _resolve_config_file_path(path, search_dir)

    try:
        with open(file_path) as f:
            data = json.load(f)
    except FileNotFoundError:
        raise FileNotFoundError(f"Config file not found: {file_path}")
    except json.JSONDecodeError as exc:
        raise ValueError(f"Invalid JSON in config file {file_path}: {exc}")

    # Extract sdk.* fields with defaults
    sdk = data.get("sdk", {}) or {}
    base_url = sdk.get("base_url", "")
    auth_type_str = sdk.get("auth_type", "")
    auth_type = AuthType(auth_type_str) if auth_type_str else AuthType.API_KEY
    timeout_ms = sdk.get("timeout_ms", 30_000)

    # Layer env var overrides on top
    env_base_url = os.environ.get("STRAIT_BASE_URL", "")
    if env_base_url:
        base_url = env_base_url

    env_auth_type = os.environ.get("STRAIT_AUTH_TYPE", "")
    if env_auth_type:
        auth_type = AuthType(env_auth_type)

    env_timeout = os.environ.get("STRAIT_TIMEOUT_MS", "")
    if env_timeout:
        try:
            timeout_ms = int(env_timeout)
        except ValueError:
            raise ValidationError(
                f'STRAIT_TIMEOUT_MS must be an integer, got "{env_timeout}"',
                issues=["STRAIT_TIMEOUT_MS is not a valid integer"],
            )

    # Token always comes from env var
    api_key = os.environ.get("STRAIT_API_KEY", "")

    return Config(
        base_url=normalize_base_url(base_url),
        auth=AuthMode(type=auth_type, token=api_key),
        timeout_ms=timeout_ms,
    )
