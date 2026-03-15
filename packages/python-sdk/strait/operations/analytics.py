"""Performance analytics operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class AnalyticsService(BaseService):
    def get_performance(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/analytics/performance", query=query)


class AsyncAnalyticsService(AsyncBaseService):
    async def get_performance(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/analytics/performance", query=query)
