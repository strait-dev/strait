"""Tests for strait._config."""


import pytest

from strait._config import (
    AuthMode,
    AuthType,
    config_from_env,
    get_authorization_header,
    normalize_base_url,
)
from strait._errors import ValidationError


class TestNormalizeBaseUrl:
    def test_strips_trailing_slash(self):
        assert normalize_base_url("https://api.example.com/") == "https://api.example.com"

    def test_strips_multiple_trailing_slashes(self):
        assert normalize_base_url("https://api.example.com///") == "https://api.example.com"

    def test_no_trailing_slash(self):
        assert normalize_base_url("https://api.example.com") == "https://api.example.com"


class TestGetAuthorizationHeader:
    def test_bearer_format(self):
        auth = AuthMode(type=AuthType.BEARER, token="my-token")
        assert get_authorization_header(auth) == "Bearer my-token"

    def test_api_key_format(self):
        auth = AuthMode(type=AuthType.API_KEY, token="key-123")
        assert get_authorization_header(auth) == "Bearer key-123"


class TestConfigFromEnv:
    def test_reads_required_vars(self, monkeypatch):
        monkeypatch.setenv("STRAIT_BASE_URL", "https://api.example.com/")
        monkeypatch.setenv("STRAIT_API_KEY", "test-key")
        cfg = config_from_env()
        assert cfg.base_url == "https://api.example.com"
        assert cfg.auth.token == "test-key"
        assert cfg.auth.type == AuthType.API_KEY
        assert cfg.timeout_ms == 30_000

    def test_missing_base_url_raises(self, monkeypatch):
        monkeypatch.delenv("STRAIT_BASE_URL", raising=False)
        monkeypatch.setenv("STRAIT_API_KEY", "key")
        with pytest.raises(ValidationError, match="STRAIT_BASE_URL"):
            config_from_env()

    def test_missing_api_key_raises(self, monkeypatch):
        monkeypatch.setenv("STRAIT_BASE_URL", "https://api.example.com")
        monkeypatch.delenv("STRAIT_API_KEY", raising=False)
        with pytest.raises(ValidationError, match="STRAIT_API_KEY"):
            config_from_env()

    def test_custom_auth_type(self, monkeypatch):
        monkeypatch.setenv("STRAIT_BASE_URL", "https://api.example.com")
        monkeypatch.setenv("STRAIT_API_KEY", "key")
        monkeypatch.setenv("STRAIT_AUTH_TYPE", "bearer")
        cfg = config_from_env()
        assert cfg.auth.type == AuthType.BEARER

    def test_custom_timeout(self, monkeypatch):
        monkeypatch.setenv("STRAIT_BASE_URL", "https://api.example.com")
        monkeypatch.setenv("STRAIT_API_KEY", "key")
        monkeypatch.setenv("STRAIT_TIMEOUT_MS", "5000")
        cfg = config_from_env()
        assert cfg.timeout_ms == 5000

    def test_invalid_timeout_raises(self, monkeypatch):
        monkeypatch.setenv("STRAIT_BASE_URL", "https://api.example.com")
        monkeypatch.setenv("STRAIT_API_KEY", "key")
        monkeypatch.setenv("STRAIT_TIMEOUT_MS", "abc")
        with pytest.raises(ValidationError, match="STRAIT_TIMEOUT_MS"):
            config_from_env()


class TestAuthType:
    def test_values(self):
        assert AuthType.BEARER == "bearer"
        assert AuthType.API_KEY == "apiKey"
        assert AuthType.RUN_TOKEN == "runToken"
