"""Event trigger management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class EventTriggersService(BaseService):
    def list_events(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/events", query=query)

    def get_event(self, event_key: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/events/{eventKey}", path_params={"eventKey": event_key},
        )

    def delete_event(self, event_key: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/events/{eventKey}", path_params={"eventKey": event_key},
        )

    def send_event(self, event_key: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/events/{eventKey}/send",
            path_params={"eventKey": event_key}, body=body,
        )

    def get_stream(self, event_key: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/events/{eventKey}/stream", path_params={"eventKey": event_key},
        )

    def send_prefix(self, prefix: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/events/prefix/{prefix}/send",
            path_params={"prefix": prefix}, body=body,
        )

    def purge_event(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/events/purge", body=body)

    def get_stat(self) -> dict[str, Any]:
        return self._request("GET", "/v1/events/stats")


class AsyncEventTriggersService(AsyncBaseService):
    async def list_events(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/events", query=query)

    async def get_event(self, event_key: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/events/{eventKey}", path_params={"eventKey": event_key},
        )

    async def delete_event(self, event_key: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/events/{eventKey}", path_params={"eventKey": event_key},
        )

    async def send_event(self, event_key: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/events/{eventKey}/send",
            path_params={"eventKey": event_key}, body=body,
        )

    async def get_stream(self, event_key: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/events/{eventKey}/stream", path_params={"eventKey": event_key},
        )

    async def send_prefix(self, prefix: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/events/prefix/{prefix}/send",
            path_params={"prefix": prefix}, body=body,
        )

    async def purge_event(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/events/purge", body=body)

    async def get_stat(self) -> dict[str, Any]:
        return await self._request("GET", "/v1/events/stats")
