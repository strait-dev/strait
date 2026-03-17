"""Tests for strait._client."""


import httpx
import pytest

from strait._client import AsyncClient, Client
from strait._config import AuthMode, AuthType
from strait._errors import NotFoundError, TransportError
from strait._middleware import Middleware


class TestClient:
    def test_constructor_with_bearer_token(self):
        c = Client(base_url="https://api.example.com", bearer_token="tok")
        assert c.base_url == "https://api.example.com"
        c.close()

    def test_constructor_with_api_key(self):
        c = Client(base_url="https://api.example.com", api_key="key")
        assert c._config.auth.type == AuthType.API_KEY
        c.close()

    def test_constructor_with_run_token(self):
        c = Client(base_url="https://api.example.com", run_token="rt")
        assert c._config.auth.type == AuthType.RUN_TOKEN
        c.close()

    def test_constructor_with_auth_mode(self):
        auth = AuthMode(type=AuthType.BEARER, token="tok")
        c = Client(base_url="https://api.example.com", auth=auth)
        assert c._config.auth.token == "tok"
        c.close()

    def test_from_env(self, monkeypatch):
        monkeypatch.setenv("STRAIT_BASE_URL", "https://api.example.com")
        monkeypatch.setenv("STRAIT_API_KEY", "env-key")
        c = Client.from_env()
        assert c.base_url == "https://api.example.com"
        assert c._config.auth.token == "env-key"
        c.close()

    def test_context_manager(self):
        with Client(base_url="https://api.example.com", api_key="key") as c:
            assert c.base_url == "https://api.example.com"

    def test_base_url_normalization(self):
        c = Client(base_url="https://api.example.com///")
        assert c.base_url == "https://api.example.com"
        c.close()

    def test_custom_http_client(self):
        transport = httpx.MockTransport(lambda r: httpx.Response(200, json={"ok": True}))
        http = httpx.Client(transport=transport)
        c = Client(base_url="https://api.example.com", api_key="key", http_client=http)
        result = c.do_request("GET", "/v1/health")
        assert result == {"ok": True}
        c.close()

    def test_do_request_sends_auth_header(self):
        def handler(request: httpx.Request) -> httpx.Response:
            assert request.headers["Authorization"] == "Bearer test-key"
            return httpx.Response(200, json={})

        transport = httpx.MockTransport(handler)
        http = httpx.Client(transport=transport)
        c = Client(base_url="https://api.example.com", api_key="test-key", http_client=http)
        c.do_request("GET", "/v1/health")
        c.close()

    def test_middleware_on_request_called(self):
        calls: list[str] = []

        def handler(request: httpx.Request) -> httpx.Response:
            return httpx.Response(200, json={})

        transport = httpx.MockTransport(handler)
        http = httpx.Client(transport=transport)
        mw = Middleware(on_request=lambda ctx: calls.append(f"{ctx.method} {ctx.url}"))
        c = Client(
            base_url="https://api.example.com", api_key="key",
            http_client=http, middleware=[mw],
        )
        c.do_request("GET", "/v1/health")
        assert len(calls) == 1
        assert "GET" in calls[0]
        c.close()

    def test_middleware_on_response_called(self):
        statuses: list[int] = []

        transport = httpx.MockTransport(lambda r: httpx.Response(200, json={}))
        http = httpx.Client(transport=transport)
        mw = Middleware(on_response=lambda ctx: statuses.append(ctx.status))
        c = Client(
            base_url="https://api.example.com", api_key="key",
            http_client=http, middleware=[mw],
        )
        c.do_request("GET", "/v1/health")
        assert statuses == [200]
        c.close()

    def test_middleware_on_error_called(self):
        errors: list[str] = []

        def handler(request: httpx.Request) -> httpx.Response:
            raise httpx.ConnectError("refused")

        transport = httpx.MockTransport(handler)
        http = httpx.Client(transport=transport)
        mw = Middleware(on_error=lambda ctx: errors.append(str(ctx.error)))
        c = Client(
            base_url="https://api.example.com", api_key="key",
            http_client=http, middleware=[mw],
        )
        with pytest.raises(TransportError):
            c.do_request("GET", "/v1/health")
        assert len(errors) == 1
        c.close()

    def test_all_services_attached(self):
        c = Client(base_url="https://api.example.com", api_key="key")
        services = [
            "health", "jobs", "runs", "workflows", "workflow_runs",
            "deployments", "environments", "secrets", "api_keys",
            "webhooks", "event_triggers", "event_sources",
            "batch_operations", "stats", "analytics", "log_drains",
            "sdk_runs", "rbac", "job_groups",
        ]
        for name in services:
            assert hasattr(c, name), f"Missing service: {name}"
        c.close()

    def test_http_error_raised(self):
        transport = httpx.MockTransport(
            lambda r: httpx.Response(404, json={"message": "not found"}),
        )
        http = httpx.Client(transport=transport)
        c = Client(base_url="https://api.example.com", api_key="key", http_client=http)
        with pytest.raises(NotFoundError):
            c.do_request("GET", "/v1/jobs/missing")
        c.close()


class TestAsyncClient:
    @pytest.mark.asyncio
    async def test_constructor_and_services(self):
        c = AsyncClient(base_url="https://api.example.com", api_key="key")
        assert c.base_url == "https://api.example.com"
        assert hasattr(c, "jobs")
        assert hasattr(c, "runs")
        await c.close()

    @pytest.mark.asyncio
    async def test_async_context_manager(self):
        async with AsyncClient(base_url="https://api.example.com", api_key="key") as c:
            assert c.base_url == "https://api.example.com"

    @pytest.mark.asyncio
    async def test_from_env(self, monkeypatch):
        monkeypatch.setenv("STRAIT_BASE_URL", "https://api.example.com")
        monkeypatch.setenv("STRAIT_API_KEY", "env-key")
        c = AsyncClient.from_env()
        assert c._config.auth.token == "env-key"
        await c.close()
