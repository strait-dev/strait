"""Log drain management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class LogDrainsService(BaseService):
    def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/log-drains", query=query)

    def create(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/log-drains", body=body)

    def get(self, drain_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/log-drains/{drainID}", path_params={"drainID": drain_id},
        )

    def update(self, drain_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "PATCH", "/v1/log-drains/{drainID}", path_params={"drainID": drain_id}, body=body,
        )

    def delete(self, drain_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/log-drains/{drainID}", path_params={"drainID": drain_id},
        )


class AsyncLogDrainsService(AsyncBaseService):
    async def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/log-drains", query=query)

    async def create(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/log-drains", body=body)

    async def get(self, drain_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/log-drains/{drainID}", path_params={"drainID": drain_id},
        )

    async def update(self, drain_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "PATCH", "/v1/log-drains/{drainID}", path_params={"drainID": drain_id}, body=body,
        )

    async def delete(self, drain_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/log-drains/{drainID}", path_params={"drainID": drain_id},
        )
