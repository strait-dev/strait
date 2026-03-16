"""Environment management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class EnvironmentsService(BaseService):
    def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/environments", query=query)

    def create(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/environments", body=body)

    def get(self, env_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/environments/{envID}", path_params={"envID": env_id},
        )

    def update(self, env_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "PATCH", "/v1/environments/{envID}", path_params={"envID": env_id}, body=body,
        )

    def delete(self, env_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/environments/{envID}", path_params={"envID": env_id},
        )

    def list_variables(self, env_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/environments/{envID}/variables", path_params={"envID": env_id},
        )


class AsyncEnvironmentsService(AsyncBaseService):
    async def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/environments", query=query)

    async def create(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/environments", body=body)

    async def get(self, env_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/environments/{envID}", path_params={"envID": env_id},
        )

    async def update(self, env_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "PATCH", "/v1/environments/{envID}", path_params={"envID": env_id}, body=body,
        )

    async def delete(self, env_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/environments/{envID}", path_params={"envID": env_id},
        )

    async def list_variables(self, env_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/environments/{envID}/variables", path_params={"envID": env_id},
        )
