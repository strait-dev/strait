"""Workflow management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class WorkflowsService(BaseService):
    def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/workflows", query=query)

    def create(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/workflows", body=body)

    def get(self, workflow_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflows/{workflowID}", path_params={"workflowID": workflow_id},
        )

    def update(self, workflow_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "PATCH", "/v1/workflows/{workflowID}",
            path_params={"workflowID": workflow_id}, body=body,
        )

    def delete(self, workflow_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/workflows/{workflowID}", path_params={"workflowID": workflow_id},
        )

    def clone(self, workflow_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflows/{workflowID}/clone",
            path_params={"workflowID": workflow_id}, body=body,
        )

    def dry_run(self, workflow_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflows/{workflowID}/dry-run",
            path_params={"workflowID": workflow_id}, body=body,
        )

    def plan(self, workflow_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflows/{workflowID}/plan",
            path_params={"workflowID": workflow_id}, body=body,
        )

    def simulate(self, workflow_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflows/{workflowID}/simulate",
            path_params={"workflowID": workflow_id}, body=body,
        )

    def trigger(self, workflow_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/workflows/{workflowID}/trigger",
            path_params={"workflowID": workflow_id}, body=body,
        )

    def get_graph(self, workflow_run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflow-runs/{workflowRunID}/graph",
            path_params={"workflowRunID": workflow_run_id},
        )

    def get_graph_by_workflow_id(self, workflow_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflows/{workflowID}/graph",
            path_params={"workflowID": workflow_id},
        )

    def list_runs(
        self, workflow_id: str, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflows/{workflowID}/runs",
            path_params={"workflowID": workflow_id}, query=query,
        )

    def list_versions(
        self, workflow_id: str, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflows/{workflowID}/versions",
            path_params={"workflowID": workflow_id}, query=query,
        )

    def get_version(self, workflow_id: str, version_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflows/{workflowID}/versions/{versionID}",
            path_params={"workflowID": workflow_id, "versionID": version_id},
        )

    def get_diff(
        self, workflow_id: str, from_version_id: str, to_version_id: str,
    ) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflows/{workflowID}/versions/{fromVersionID}/diff/{toVersionID}",
            path_params={
                "workflowID": workflow_id,
                "fromVersionID": from_version_id,
                "toVersionID": to_version_id,
            },
        )

    def get_policy(self, project_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflow-policies/{projectID}",
            path_params={"projectID": project_id},
        )

    def upsert_policy(self, project_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "PUT", "/v1/workflow-policies/{projectID}",
            path_params={"projectID": project_id}, body=body,
        )

    def get_explain(self, workflow_run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflow-runs/{workflowRunID}/explain",
            path_params={"workflowRunID": workflow_run_id},
        )

    def list_labels(self, workflow_run_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflow-runs/{workflowRunID}/labels",
            path_params={"workflowRunID": workflow_run_id},
        )

    def get_impact(self, workflow_id: str, version_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflows/{workflowID}/versions/{versionID}/impact",
            path_params={"workflowID": workflow_id, "versionID": version_id},
        )

    def list_steps_by_version(self, workflow_id: str, version_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/workflows/{workflowID}/versions/{versionID}/steps",
            path_params={"workflowID": workflow_id, "versionID": version_id},
        )


class AsyncWorkflowsService(AsyncBaseService):
    async def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/workflows", query=query)

    async def create(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/workflows", body=body)

    async def get(self, workflow_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflows/{workflowID}", path_params={"workflowID": workflow_id},
        )

    async def update(self, workflow_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "PATCH", "/v1/workflows/{workflowID}",
            path_params={"workflowID": workflow_id}, body=body,
        )

    async def delete(self, workflow_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/workflows/{workflowID}", path_params={"workflowID": workflow_id},
        )

    async def clone(self, workflow_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflows/{workflowID}/clone",
            path_params={"workflowID": workflow_id}, body=body,
        )

    async def dry_run(self, workflow_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflows/{workflowID}/dry-run",
            path_params={"workflowID": workflow_id}, body=body,
        )

    async def plan(self, workflow_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflows/{workflowID}/plan",
            path_params={"workflowID": workflow_id}, body=body,
        )

    async def simulate(self, workflow_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflows/{workflowID}/simulate",
            path_params={"workflowID": workflow_id}, body=body,
        )

    async def trigger(self, workflow_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/workflows/{workflowID}/trigger",
            path_params={"workflowID": workflow_id}, body=body,
        )

    async def get_graph(self, workflow_run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflow-runs/{workflowRunID}/graph",
            path_params={"workflowRunID": workflow_run_id},
        )

    async def get_graph_by_workflow_id(self, workflow_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflows/{workflowID}/graph",
            path_params={"workflowID": workflow_id},
        )

    async def list_runs(
        self, workflow_id: str, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflows/{workflowID}/runs",
            path_params={"workflowID": workflow_id}, query=query,
        )

    async def list_versions(
        self, workflow_id: str, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflows/{workflowID}/versions",
            path_params={"workflowID": workflow_id}, query=query,
        )

    async def get_version(self, workflow_id: str, version_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflows/{workflowID}/versions/{versionID}",
            path_params={"workflowID": workflow_id, "versionID": version_id},
        )

    async def get_diff(
        self, workflow_id: str, from_version_id: str, to_version_id: str,
    ) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflows/{workflowID}/versions/{fromVersionID}/diff/{toVersionID}",
            path_params={
                "workflowID": workflow_id,
                "fromVersionID": from_version_id,
                "toVersionID": to_version_id,
            },
        )

    async def get_policy(self, project_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflow-policies/{projectID}",
            path_params={"projectID": project_id},
        )

    async def upsert_policy(self, project_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "PUT", "/v1/workflow-policies/{projectID}",
            path_params={"projectID": project_id}, body=body,
        )

    async def get_explain(self, workflow_run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflow-runs/{workflowRunID}/explain",
            path_params={"workflowRunID": workflow_run_id},
        )

    async def list_labels(self, workflow_run_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflow-runs/{workflowRunID}/labels",
            path_params={"workflowRunID": workflow_run_id},
        )

    async def get_impact(self, workflow_id: str, version_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflows/{workflowID}/versions/{versionID}/impact",
            path_params={"workflowID": workflow_id, "versionID": version_id},
        )

    async def list_steps_by_version(self, workflow_id: str, version_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/workflows/{workflowID}/versions/{versionID}/steps",
            path_params={"workflowID": workflow_id, "versionID": version_id},
        )
