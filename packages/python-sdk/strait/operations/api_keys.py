"""API key management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class APIKeysService(BaseService):
    def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/api-keys", query=query)

    def create(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/api-keys", body=body)

    def delete(self, key_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/api-keys/{keyID}", path_params={"keyID": key_id},
        )

    def rotate(self, key_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/api-keys/{keyID}/rotate", path_params={"keyID": key_id}, body=body,
        )


class AsyncAPIKeysService(AsyncBaseService):
    async def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/api-keys", query=query)

    async def create(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/api-keys", body=body)

    async def delete(self, key_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/api-keys/{keyID}", path_params={"keyID": key_id},
        )

    async def rotate(self, key_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/api-keys/{keyID}/rotate", path_params={"keyID": key_id}, body=body,
        )
