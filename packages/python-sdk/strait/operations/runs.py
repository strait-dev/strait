"""Run management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class RunsService(BaseService):
    def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/runs", query=query)

    def get(self, run_id: str) -> dict[str, Any]:
        return self._request("GET", "/v1/runs/{runID}", path_params={"runID": run_id})

    def delete(self, run_id: str) -> dict[str, Any]:
        return self._request("DELETE", "/v1/runs/{runID}", path_params={"runID": run_id})

    def list_checkpoints(self, run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/runs/{runID}/checkpoints", path_params={"runID": run_id},
        )

    def get_children(
        self, run_id: str, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/runs/{runID}/children", path_params={"runID": run_id}, query=query,
        )

    def debug(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/runs/{runID}/debug", path_params={"runID": run_id}, body=body,
        )

    def get_debug_bundle(self, run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/runs/{runID}/debug-bundle", path_params={"runID": run_id},
        )

    def list_dependency_status(self, run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/runs/{runID}/dependency-status", path_params={"runID": run_id},
        )

    def dlq_replay(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/runs/{runID}/dlq-replay", path_params={"runID": run_id}, body=body,
        )

    def list_events(
        self, run_id: str, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/runs/{runID}/events", path_params={"runID": run_id}, query=query,
        )

    def delete_idempotency_key(self, run_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/runs/{runID}/idempotency-key", path_params={"runID": run_id},
        )

    def get_lineage(self, run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/runs/{runID}/lineage", path_params={"runID": run_id},
        )

    def list_outputs(self, run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/runs/{runID}/outputs", path_params={"runID": run_id},
        )

    def replay(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/runs/{runID}/replay", path_params={"runID": run_id}, body=body,
        )

    def reschedule(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/runs/{runID}/reschedule", path_params={"runID": run_id}, body=body,
        )

    def get_stream(self, run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/runs/{runID}/stream", path_params={"runID": run_id},
        )

    def list_tool_calls(self, run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/runs/{runID}/tool-calls", path_params={"runID": run_id},
        )

    def get_usage(self, run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/runs/{runID}/usage", path_params={"runID": run_id},
        )

    def bulk_cancel(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/runs/bulk-cancel", body=body)

    def bulk_cancel_all(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/runs/bulk-cancel-all", body=body)

    def bulk_dlq_replay(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/runs/bulk-dlq-replay", body=body)

    def bulk_replay(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/runs/bulk-replay", body=body)

    def get_dlq(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/runs/dlq", query=query)


class AsyncRunsService(AsyncBaseService):
    async def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/runs", query=query)

    async def get(self, run_id: str) -> dict[str, Any]:
        return await self._request("GET", "/v1/runs/{runID}", path_params={"runID": run_id})

    async def delete(self, run_id: str) -> dict[str, Any]:
        return await self._request("DELETE", "/v1/runs/{runID}", path_params={"runID": run_id})

    async def list_checkpoints(self, run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/runs/{runID}/checkpoints", path_params={"runID": run_id},
        )

    async def get_children(
        self, run_id: str, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/runs/{runID}/children", path_params={"runID": run_id}, query=query,
        )

    async def debug(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/runs/{runID}/debug", path_params={"runID": run_id}, body=body,
        )

    async def get_debug_bundle(self, run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/runs/{runID}/debug-bundle", path_params={"runID": run_id},
        )

    async def list_dependency_status(self, run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/runs/{runID}/dependency-status", path_params={"runID": run_id},
        )

    async def dlq_replay(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/runs/{runID}/dlq-replay", path_params={"runID": run_id}, body=body,
        )

    async def list_events(
        self, run_id: str, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/runs/{runID}/events", path_params={"runID": run_id}, query=query,
        )

    async def delete_idempotency_key(self, run_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/runs/{runID}/idempotency-key", path_params={"runID": run_id},
        )

    async def get_lineage(self, run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/runs/{runID}/lineage", path_params={"runID": run_id},
        )

    async def list_outputs(self, run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/runs/{runID}/outputs", path_params={"runID": run_id},
        )

    async def replay(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/runs/{runID}/replay", path_params={"runID": run_id}, body=body,
        )

    async def reschedule(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/runs/{runID}/reschedule", path_params={"runID": run_id}, body=body,
        )

    async def get_stream(self, run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/runs/{runID}/stream", path_params={"runID": run_id},
        )

    async def list_tool_calls(self, run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/runs/{runID}/tool-calls", path_params={"runID": run_id},
        )

    async def get_usage(self, run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/runs/{runID}/usage", path_params={"runID": run_id},
        )

    async def bulk_cancel(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/runs/bulk-cancel", body=body)

    async def bulk_cancel_all(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/runs/bulk-cancel-all", body=body)

    async def bulk_dlq_replay(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/runs/bulk-dlq-replay", body=body)

    async def bulk_replay(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/runs/bulk-replay", body=body)

    async def get_dlq(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/runs/dlq", query=query)
