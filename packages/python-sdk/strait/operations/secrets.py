"""Secret management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class SecretsService(BaseService):
    def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/secrets", query=query)

    def create(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/secrets", body=body)

    def delete(self, secret_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/secrets/{secretID}", path_params={"secretID": secret_id},
        )


class AsyncSecretsService(AsyncBaseService):
    async def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/secrets", query=query)

    async def create(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/secrets", body=body)

    async def delete(self, secret_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/secrets/{secretID}", path_params={"secretID": secret_id},
        )
