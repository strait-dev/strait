"""Webhook management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class WebhooksService(BaseService):
    def list_subscriptions(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/webhooks/subscriptions", query=query)

    def create_subscription(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/webhooks/subscriptions", body=body)

    def delete_subscription(self, id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/webhooks/subscriptions/{id}", path_params={"id": id},
        )

    def list_deliveries(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/webhooks/deliveries", query=query)

    def get_delivery(self, id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/webhooks/deliveries/{id}", path_params={"id": id},
        )

    def retry_delivery(self, id: str) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/webhooks/deliveries/{id}/retry", path_params={"id": id},
        )


class AsyncWebhooksService(AsyncBaseService):
    async def list_subscriptions(
        self, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return await self._request("GET", "/v1/webhooks/subscriptions", query=query)

    async def create_subscription(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/webhooks/subscriptions", body=body)

    async def delete_subscription(self, id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/webhooks/subscriptions/{id}", path_params={"id": id},
        )

    async def list_deliveries(
        self, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return await self._request("GET", "/v1/webhooks/deliveries", query=query)

    async def get_delivery(self, id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/webhooks/deliveries/{id}", path_params={"id": id},
        )

    async def retry_delivery(self, id: str) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/webhooks/deliveries/{id}/retry", path_params={"id": id},
        )
