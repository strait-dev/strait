"""SDK run-token operations for executor use."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class SDKRunsService(BaseService):
    def annotate_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/annotate", path_params={"runID": run_id}, body=body,
        )

    def checkpoint_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/checkpoint", path_params={"runID": run_id}, body=body,
        )

    def complete_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/complete", path_params={"runID": run_id}, body=body,
        )

    def continue_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/continue", path_params={"runID": run_id}, body=body,
        )

    def fail_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/fail", path_params={"runID": run_id}, body=body,
        )

    def heartbeat_run(self, run_id: str) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/heartbeat", path_params={"runID": run_id},
        )

    def log_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/log", path_params={"runID": run_id}, body=body,
        )

    def output_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/output", path_params={"runID": run_id}, body=body,
        )

    def progress_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/progress", path_params={"runID": run_id}, body=body,
        )

    def spawn_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/spawn", path_params={"runID": run_id}, body=body,
        )

    def tool_call_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/tool-call", path_params={"runID": run_id}, body=body,
        )

    def usage_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/usage", path_params={"runID": run_id}, body=body,
        )

    def wait_for_event_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/sdk/v1/runs/{runID}/wait-for-event",
            path_params={"runID": run_id}, body=body,
        )


class AsyncSDKRunsService(AsyncBaseService):
    async def annotate_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/annotate", path_params={"runID": run_id}, body=body,
        )

    async def checkpoint_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/checkpoint", path_params={"runID": run_id}, body=body,
        )

    async def complete_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/complete", path_params={"runID": run_id}, body=body,
        )

    async def continue_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/continue", path_params={"runID": run_id}, body=body,
        )

    async def fail_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/fail", path_params={"runID": run_id}, body=body,
        )

    async def heartbeat_run(self, run_id: str) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/heartbeat", path_params={"runID": run_id},
        )

    async def log_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/log", path_params={"runID": run_id}, body=body,
        )

    async def output_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/output", path_params={"runID": run_id}, body=body,
        )

    async def progress_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/progress", path_params={"runID": run_id}, body=body,
        )

    async def spawn_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/spawn", path_params={"runID": run_id}, body=body,
        )

    async def tool_call_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/tool-call", path_params={"runID": run_id}, body=body,
        )

    async def usage_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/usage", path_params={"runID": run_id}, body=body,
        )

    async def wait_for_event_run(self, run_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/sdk/v1/runs/{runID}/wait-for-event",
            path_params={"runID": run_id}, body=body,
        )
