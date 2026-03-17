"""Event source management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class EventSourcesService(BaseService):
    def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/event-sources", query=query)

    def create(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/event-sources", body=body)

    def get(self, source_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/event-sources/{sourceID}", path_params={"sourceID": source_id},
        )

    def update(self, source_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "PATCH", "/v1/event-sources/{sourceID}",
            path_params={"sourceID": source_id}, body=body,
        )

    def delete(self, source_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/event-sources/{sourceID}", path_params={"sourceID": source_id},
        )

    def subscribe(self, source_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/event-sources/{sourceID}/subscribe",
            path_params={"sourceID": source_id}, body=body,
        )

    def list_subscriptions(self, source_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/event-sources/{sourceID}/subscriptions",
            path_params={"sourceID": source_id},
        )

    def delete_subscription(self, source_id: str, sub_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/event-sources/{sourceID}/subscriptions/{subID}",
            path_params={"sourceID": source_id, "subID": sub_id},
        )

    def dispatch_event(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/events/dispatch", body=body)


class AsyncEventSourcesService(AsyncBaseService):
    async def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/event-sources", query=query)

    async def create(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/event-sources", body=body)

    async def get(self, source_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/event-sources/{sourceID}", path_params={"sourceID": source_id},
        )

    async def update(self, source_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "PATCH", "/v1/event-sources/{sourceID}",
            path_params={"sourceID": source_id}, body=body,
        )

    async def delete(self, source_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/event-sources/{sourceID}", path_params={"sourceID": source_id},
        )

    async def subscribe(self, source_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/event-sources/{sourceID}/subscribe",
            path_params={"sourceID": source_id}, body=body,
        )

    async def list_subscriptions(self, source_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/event-sources/{sourceID}/subscriptions",
            path_params={"sourceID": source_id},
        )

    async def delete_subscription(self, source_id: str, sub_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/event-sources/{sourceID}/subscriptions/{subID}",
            path_params={"sourceID": source_id, "subID": sub_id},
        )

    async def dispatch_event(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/events/dispatch", body=body)
