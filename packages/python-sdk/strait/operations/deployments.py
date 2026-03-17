"""Deployment management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class DeploymentsService(BaseService):
    def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/deployments", query=query)

    def create(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/deployments", body=body)

    def finalize(self, deployment_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/deployments/{deploymentID}/finalize",
            path_params={"deploymentID": deployment_id}, body=body,
        )

    def promote(self, deployment_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/deployments/{deploymentID}/promote",
            path_params={"deploymentID": deployment_id}, body=body,
        )

    def rollback(self, deployment_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/deployments/{deploymentID}/rollback",
            path_params={"deploymentID": deployment_id}, body=body,
        )


class AsyncDeploymentsService(AsyncBaseService):
    async def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/deployments", query=query)

    async def create(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/deployments", body=body)

    async def finalize(self, deployment_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/deployments/{deploymentID}/finalize",
            path_params={"deploymentID": deployment_id}, body=body,
        )

    async def promote(self, deployment_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/deployments/{deploymentID}/promote",
            path_params={"deploymentID": deployment_id}, body=body,
        )

    async def rollback(self, deployment_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/deployments/{deploymentID}/rollback",
            path_params={"deploymentID": deployment_id}, body=body,
        )
