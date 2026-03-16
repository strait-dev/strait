"""Workflow run management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class WorkflowRunsService(BaseService):
    def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/workflow-runs", query=query)

    def get(self, workflow_run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflow-runs/{workflowRunID}",
            path_params={"workflowRunID": workflow_run_id},
        )

    def delete(self, workflow_run_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/workflow-runs/{workflowRunID}",
            path_params={"workflowRunID": workflow_run_id},
        )

    def pause(self, workflow_run_id: str) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/pause",
            path_params={"workflowRunID": workflow_run_id},
        )

    def resume(self, workflow_run_id: str) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/resume",
            path_params={"workflowRunID": workflow_run_id},
        )

    def retry(self, workflow_run_id: str) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/retry",
            path_params={"workflowRunID": workflow_run_id},
        )

    def list_steps(self, workflow_run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflow-runs/{workflowRunID}/steps",
            path_params={"workflowRunID": workflow_run_id},
        )

    def approve_step(
        self, workflow_run_id: str, step_ref: str, body: Any,
    ) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/approve",
            path_params={"workflowRunID": workflow_run_id, "stepRef": step_ref},
            body=body,
        )

    def retry_step(self, workflow_run_id: str, step_ref: str) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/retry",
            path_params={"workflowRunID": workflow_run_id, "stepRef": step_ref},
        )

    def skip_step(self, workflow_run_id: str, step_ref: str) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/skip",
            path_params={"workflowRunID": workflow_run_id, "stepRef": step_ref},
        )

    def force_complete_step(
        self, workflow_run_id: str, step_ref: str, body: Any,
    ) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/force-complete",
            path_params={"workflowRunID": workflow_run_id, "stepRef": step_ref},
            body=body,
        )

    def replay_subtree_step(self, workflow_run_id: str, step_ref: str) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/replay-subtree",
            path_params={"workflowRunID": workflow_run_id, "stepRef": step_ref},
        )

    def bulk_cancel(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/workflow-runs/bulk-cancel", body=body)

    def bulk_replay(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/workflow-runs/bulk-replay", body=body)


class AsyncWorkflowRunsService(AsyncBaseService):
    async def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/workflow-runs", query=query)

    async def get(self, workflow_run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflow-runs/{workflowRunID}",
            path_params={"workflowRunID": workflow_run_id},
        )

    async def delete(self, workflow_run_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/workflow-runs/{workflowRunID}",
            path_params={"workflowRunID": workflow_run_id},
        )

    async def pause(self, workflow_run_id: str) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/pause",
            path_params={"workflowRunID": workflow_run_id},
        )

    async def resume(self, workflow_run_id: str) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/resume",
            path_params={"workflowRunID": workflow_run_id},
        )

    async def retry(self, workflow_run_id: str) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/retry",
            path_params={"workflowRunID": workflow_run_id},
        )

    async def list_steps(self, workflow_run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflow-runs/{workflowRunID}/steps",
            path_params={"workflowRunID": workflow_run_id},
        )

    async def approve_step(
        self, workflow_run_id: str, step_ref: str, body: Any,
    ) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/approve",
            path_params={"workflowRunID": workflow_run_id, "stepRef": step_ref},
            body=body,
        )

    async def retry_step(self, workflow_run_id: str, step_ref: str) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/retry",
            path_params={"workflowRunID": workflow_run_id, "stepRef": step_ref},
        )

    async def skip_step(self, workflow_run_id: str, step_ref: str) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/skip",
            path_params={"workflowRunID": workflow_run_id, "stepRef": step_ref},
        )

    async def force_complete_step(
        self, workflow_run_id: str, step_ref: str, body: Any,
    ) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/force-complete",
            path_params={"workflowRunID": workflow_run_id, "stepRef": step_ref},
            body=body,
        )

    async def replay_subtree_step(
        self, workflow_run_id: str, step_ref: str,
    ) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/replay-subtree",
            path_params={"workflowRunID": workflow_run_id, "stepRef": step_ref},
        )

    async def bulk_cancel(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/workflow-runs/bulk-cancel", body=body)

    async def bulk_replay(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/workflow-runs/bulk-replay", body=body)
