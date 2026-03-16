"""Base service classes for domain operations."""

from __future__ import annotations

from typing import Any, Protocol

from strait._http import substitute_path_params


class Requester(Protocol):
    """Protocol abstracting the Client.do_request method."""

    def do_request(
        self,
        method: str,
        path: str,
        *,
        query: dict[str, str] | None = None,
        headers: dict[str, str] | None = None,
        body: Any = None,
    ) -> dict[str, Any]: ...


class AsyncRequester(Protocol):
    """Protocol abstracting the AsyncClient.do_request method."""

    async def do_request(
        self,
        method: str,
        path: str,
        *,
        query: dict[str, str] | None = None,
        headers: dict[str, str] | None = None,
        body: Any = None,
    ) -> dict[str, Any]: ...


class BaseService:
    """Base class for sync domain services."""

    def __init__(self, requester: Requester) -> None:
        self._r = requester

    def _request(
        self,
        method: str,
        path: str,
        *,
        path_params: dict[str, str] | None = None,
        query: dict[str, str] | None = None,
        headers: dict[str, str] | None = None,
        body: Any = None,
    ) -> dict[str, Any]:
        if path_params:
            path = substitute_path_params(path, path_params)
        return self._r.do_request(method, path, query=query, headers=headers, body=body)

    def _request_no_content(
        self,
        method: str,
        path: str,
        *,
        path_params: dict[str, str] | None = None,
    ) -> None:
        if path_params:
            path = substitute_path_params(path, path_params)
        self._r.do_request(method, path)


class AsyncBaseService:
    """Base class for async domain services."""

    def __init__(self, requester: AsyncRequester) -> None:
        self._r = requester

    async def _request(
        self,
        method: str,
        path: str,
        *,
        path_params: dict[str, str] | None = None,
        query: dict[str, str] | None = None,
        headers: dict[str, str] | None = None,
        body: Any = None,
    ) -> dict[str, Any]:
        if path_params:
            path = substitute_path_params(path, path_params)
        return await self._r.do_request(method, path, query=query, headers=headers, body=body)

    async def _request_no_content(
        self,
        method: str,
        path: str,
        *,
        path_params: dict[str, str] | None = None,
    ) -> None:
        if path_params:
            path = substitute_path_params(path, path_params)
        await self._r.do_request(method, path)
