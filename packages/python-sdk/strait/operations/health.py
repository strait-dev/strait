"""Health check operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class HealthService(BaseService):
    def list(self) -> dict[str, Any]:
        return self._request("GET", "/health")

    def get_ready(self) -> dict[str, Any]:
        return self._request("GET", "/health/ready")

    def list_metrics(self) -> dict[str, Any]:
        return self._request("GET", "/metrics")


class AsyncHealthService(AsyncBaseService):
    async def list(self) -> dict[str, Any]:
        return await self._request("GET", "/health")

    async def get_ready(self) -> dict[str, Any]:
        return await self._request("GET", "/health/ready")

    async def list_metrics(self) -> dict[str, Any]:
        return await self._request("GET", "/metrics")
