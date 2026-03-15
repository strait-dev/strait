"""Strait SDK Client and AsyncClient."""

from __future__ import annotations

import time
from typing import Any

import httpx

from strait._config import (
    AuthMode,
    AuthType,
    Config,
    config_from_env,
    get_authorization_header,
    normalize_base_url,
)
from strait._errors import DecodeError, TransportError, map_http_error
from strait._middleware import (
    Middleware,
    MiddlewareErrorContext,
    MiddlewareRequestContext,
    MiddlewareResponseContext,
)
from strait._types import JsonDict
from strait.operations.analytics import AnalyticsService, AsyncAnalyticsService
from strait.operations.api_keys import APIKeysService, AsyncAPIKeysService
from strait.operations.batch_operations import AsyncBatchOperationsService, BatchOperationsService
from strait.operations.deployments import AsyncDeploymentsService, DeploymentsService
from strait.operations.environments import AsyncEnvironmentsService, EnvironmentsService
from strait.operations.event_sources import AsyncEventSourcesService, EventSourcesService
from strait.operations.event_triggers import AsyncEventTriggersService, EventTriggersService
from strait.operations.health import AsyncHealthService, HealthService
from strait.operations.job_groups import AsyncJobGroupsService, JobGroupsService
from strait.operations.jobs import AsyncJobsService, JobsService
from strait.operations.log_drains import AsyncLogDrainsService, LogDrainsService
from strait.operations.rbac import AsyncRBACService, RBACService
from strait.operations.runs import AsyncRunsService, RunsService
from strait.operations.sdk_runs import AsyncSDKRunsService, SDKRunsService
from strait.operations.secrets import AsyncSecretsService, SecretsService
from strait.operations.stats import AsyncStatsService, StatsService
from strait.operations.webhooks import AsyncWebhooksService, WebhooksService
from strait.operations.workflow_runs import AsyncWorkflowRunsService, WorkflowRunsService
from strait.operations.workflows import AsyncWorkflowsService, WorkflowsService


def _resolve_auth(
    bearer_token: str | None,
    api_key: str | None,
    run_token: str | None,
    auth: AuthMode | None,
) -> AuthMode | None:
    if auth is not None:
        return auth
    if bearer_token is not None:
        return AuthMode(type=AuthType.BEARER, token=bearer_token)
    if api_key is not None:
        return AuthMode(type=AuthType.API_KEY, token=api_key)
    if run_token is not None:
        return AuthMode(type=AuthType.RUN_TOKEN, token=run_token)
    return None


class Client:
    """Synchronous Strait API client."""

    def __init__(
        self,
        *,
        base_url: str = "",
        bearer_token: str | None = None,
        api_key: str | None = None,
        run_token: str | None = None,
        auth: AuthMode | None = None,
        default_headers: dict[str, str] | None = None,
        timeout_ms: int = 30_000,
        http_client: httpx.Client | None = None,
        middleware: list[Middleware] | None = None,
    ) -> None:
        resolved_auth = _resolve_auth(bearer_token, api_key, run_token, auth)
        if resolved_auth is None:
            resolved_auth = AuthMode(type=AuthType.API_KEY, token="")

        self._config = Config(
            base_url=normalize_base_url(base_url),
            auth=resolved_auth,
            default_headers=default_headers or {},
            timeout_ms=timeout_ms,
        )
        self._middleware = middleware or []
        self._owns_client = http_client is None
        self._http = http_client or httpx.Client(timeout=timeout_ms / 1000.0)

        self.health = HealthService(self)
        self.jobs = JobsService(self)
        self.runs = RunsService(self)
        self.workflows = WorkflowsService(self)
        self.workflow_runs = WorkflowRunsService(self)
        self.deployments = DeploymentsService(self)
        self.environments = EnvironmentsService(self)
        self.secrets = SecretsService(self)
        self.api_keys = APIKeysService(self)
        self.webhooks = WebhooksService(self)
        self.event_triggers = EventTriggersService(self)
        self.event_sources = EventSourcesService(self)
        self.batch_operations = BatchOperationsService(self)
        self.stats = StatsService(self)
        self.analytics = AnalyticsService(self)
        self.log_drains = LogDrainsService(self)
        self.sdk_runs = SDKRunsService(self)
        self.rbac = RBACService(self)
        self.job_groups = JobGroupsService(self)

    @classmethod
    def from_env(cls, **overrides: Any) -> Client:
        cfg = config_from_env()
        return cls(
            base_url=overrides.pop("base_url", cfg.base_url),
            auth=overrides.pop("auth", cfg.auth),
            timeout_ms=overrides.pop("timeout_ms", cfg.timeout_ms),
            **overrides,
        )

    @property
    def base_url(self) -> str:
        return self._config.base_url

    def do_request(
        self,
        method: str,
        path: str,
        *,
        query: dict[str, str] | None = None,
        headers: dict[str, str] | None = None,
        body: Any = None,
    ) -> JsonDict:
        url = self._config.base_url + path

        req_headers: dict[str, str] = {
            "Content-Type": "application/json",
            "Accept": "application/json",
            "Authorization": get_authorization_header(self._config.auth),
        }
        req_headers.update(self._config.default_headers)
        if headers:
            req_headers.update(headers)

        for mw in self._middleware:
            if mw.on_request:
                mw.on_request(MiddlewareRequestContext(
                    method=method, url=url, headers=dict(req_headers),
                ))

        start = time.monotonic()
        try:
            response = self._http.request(
                method,
                url,
                params=query,
                headers=req_headers,
                json=body if body is not None else None,
                content=None if body is not None else None,
            )
        except httpx.HTTPError as exc:
            for mw in self._middleware:
                if mw.on_error:
                    mw.on_error(MiddlewareErrorContext(
                        method=method, url=url, error=exc,
                    ))
            raise TransportError(f"request failed: {exc}", cause=exc)

        duration_ms = int((time.monotonic() - start) * 1000)
        for mw in self._middleware:
            if mw.on_response:
                mw.on_response(MiddlewareResponseContext(
                    method=method, url=url, status=response.status_code,
                    duration_ms=duration_ms,
                ))

        resp_body = response.text

        if response.status_code < 200 or response.status_code >= 300:
            err_body: Any = None
            try:
                err_body = response.json()
            except Exception:
                pass
            msg = f"HTTP {response.status_code}: {method} {path}"
            if isinstance(err_body, dict) and "message" in err_body:
                msg = err_body["message"]
            raise map_http_error(response.status_code, msg, err_body)

        if not resp_body:
            return {}

        try:
            return response.json()  # type: ignore[no-any-return]
        except Exception as exc:
            raise DecodeError(f"failed to decode response: {exc}", body=resp_body, cause=exc)

    def close(self) -> None:
        if self._owns_client:
            self._http.close()

    def __enter__(self) -> Client:
        return self

    def __exit__(self, *_: Any) -> None:
        self.close()


class AsyncClient:
    """Asynchronous Strait API client."""

    def __init__(
        self,
        *,
        base_url: str = "",
        bearer_token: str | None = None,
        api_key: str | None = None,
        run_token: str | None = None,
        auth: AuthMode | None = None,
        default_headers: dict[str, str] | None = None,
        timeout_ms: int = 30_000,
        http_client: httpx.AsyncClient | None = None,
        middleware: list[Middleware] | None = None,
    ) -> None:
        resolved_auth = _resolve_auth(bearer_token, api_key, run_token, auth)
        if resolved_auth is None:
            resolved_auth = AuthMode(type=AuthType.API_KEY, token="")

        self._config = Config(
            base_url=normalize_base_url(base_url),
            auth=resolved_auth,
            default_headers=default_headers or {},
            timeout_ms=timeout_ms,
        )
        self._middleware = middleware or []
        self._owns_client = http_client is None
        self._http = http_client or httpx.AsyncClient(timeout=timeout_ms / 1000.0)

        self.health = AsyncHealthService(self)
        self.jobs = AsyncJobsService(self)
        self.runs = AsyncRunsService(self)
        self.workflows = AsyncWorkflowsService(self)
        self.workflow_runs = AsyncWorkflowRunsService(self)
        self.deployments = AsyncDeploymentsService(self)
        self.environments = AsyncEnvironmentsService(self)
        self.secrets = AsyncSecretsService(self)
        self.api_keys = AsyncAPIKeysService(self)
        self.webhooks = AsyncWebhooksService(self)
        self.event_triggers = AsyncEventTriggersService(self)
        self.event_sources = AsyncEventSourcesService(self)
        self.batch_operations = AsyncBatchOperationsService(self)
        self.stats = AsyncStatsService(self)
        self.analytics = AsyncAnalyticsService(self)
        self.log_drains = AsyncLogDrainsService(self)
        self.sdk_runs = AsyncSDKRunsService(self)
        self.rbac = AsyncRBACService(self)
        self.job_groups = AsyncJobGroupsService(self)

    @classmethod
    def from_env(cls, **overrides: Any) -> AsyncClient:
        cfg = config_from_env()
        return cls(
            base_url=overrides.pop("base_url", cfg.base_url),
            auth=overrides.pop("auth", cfg.auth),
            timeout_ms=overrides.pop("timeout_ms", cfg.timeout_ms),
            **overrides,
        )

    @property
    def base_url(self) -> str:
        return self._config.base_url

    async def do_request(
        self,
        method: str,
        path: str,
        *,
        query: dict[str, str] | None = None,
        headers: dict[str, str] | None = None,
        body: Any = None,
    ) -> JsonDict:
        url = self._config.base_url + path

        req_headers: dict[str, str] = {
            "Content-Type": "application/json",
            "Accept": "application/json",
            "Authorization": get_authorization_header(self._config.auth),
        }
        req_headers.update(self._config.default_headers)
        if headers:
            req_headers.update(headers)

        for mw in self._middleware:
            if mw.on_request:
                mw.on_request(MiddlewareRequestContext(
                    method=method, url=url, headers=dict(req_headers),
                ))

        start = time.monotonic()
        try:
            response = await self._http.request(
                method,
                url,
                params=query,
                headers=req_headers,
                json=body if body is not None else None,
                content=None if body is not None else None,
            )
        except httpx.HTTPError as exc:
            for mw in self._middleware:
                if mw.on_error:
                    mw.on_error(MiddlewareErrorContext(
                        method=method, url=url, error=exc,
                    ))
            raise TransportError(f"request failed: {exc}", cause=exc)

        duration_ms = int((time.monotonic() - start) * 1000)
        for mw in self._middleware:
            if mw.on_response:
                mw.on_response(MiddlewareResponseContext(
                    method=method, url=url, status=response.status_code,
                    duration_ms=duration_ms,
                ))

        resp_body = response.text

        if response.status_code < 200 or response.status_code >= 300:
            err_body: Any = None
            try:
                err_body = response.json()
            except Exception:
                pass
            msg = f"HTTP {response.status_code}: {method} {path}"
            if isinstance(err_body, dict) and "message" in err_body:
                msg = err_body["message"]
            raise map_http_error(response.status_code, msg, err_body)

        if not resp_body:
            return {}

        try:
            return response.json()  # type: ignore[no-any-return]
        except Exception as exc:
            raise DecodeError(f"failed to decode response: {exc}", body=resp_body, cause=exc)

    async def close(self) -> None:
        if self._owns_client:
            await self._http.aclose()

    async def __aenter__(self) -> AsyncClient:
        return self

    async def __aexit__(self, *_: Any) -> None:
        await self.close()
