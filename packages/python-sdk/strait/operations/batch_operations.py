"""Batch operation management."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class BatchOperationsService(BaseService):
    def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/batch-operations", query=query)

    def get(self, batch_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/batch-operations/{batchID}", path_params={"batchID": batch_id},
        )


class AsyncBatchOperationsService(AsyncBaseService):
    async def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/batch-operations", query=query)

    async def get(self, batch_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/batch-operations/{batchID}", path_params={"batchID": batch_id},
        )
