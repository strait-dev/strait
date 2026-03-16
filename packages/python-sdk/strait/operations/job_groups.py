"""Job group management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class JobGroupsService(BaseService):
    def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/job-groups", query=query)

    def create(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/job-groups", body=body)

    def get(self, group_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/job-groups/{groupID}", path_params={"groupID": group_id},
        )

    def update(self, group_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "PATCH", "/v1/job-groups/{groupID}", path_params={"groupID": group_id}, body=body,
        )

    def delete(self, group_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/job-groups/{groupID}", path_params={"groupID": group_id},
        )

    def list_jobs(
        self, group_id: str, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/job-groups/{groupID}/jobs",
            path_params={"groupID": group_id}, query=query,
        )

    def pause_all(self, group_id: str) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/job-groups/{groupID}/pause-all",
            path_params={"groupID": group_id},
        )

    def resume_all(self, group_id: str) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/job-groups/{groupID}/resume-all",
            path_params={"groupID": group_id},
        )

    def get_stats(self, group_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/job-groups/{groupID}/stats", path_params={"groupID": group_id},
        )


class AsyncJobGroupsService(AsyncBaseService):
    async def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/job-groups", query=query)

    async def create(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/job-groups", body=body)

    async def get(self, group_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/job-groups/{groupID}", path_params={"groupID": group_id},
        )

    async def update(self, group_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "PATCH", "/v1/job-groups/{groupID}", path_params={"groupID": group_id}, body=body,
        )

    async def delete(self, group_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/job-groups/{groupID}", path_params={"groupID": group_id},
        )

    async def list_jobs(
        self, group_id: str, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/job-groups/{groupID}/jobs",
            path_params={"groupID": group_id}, query=query,
        )

    async def pause_all(self, group_id: str) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/job-groups/{groupID}/pause-all",
            path_params={"groupID": group_id},
        )

    async def resume_all(self, group_id: str) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/job-groups/{groupID}/resume-all",
            path_params={"groupID": group_id},
        )

    async def get_stats(self, group_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/job-groups/{groupID}/stats", path_params={"groupID": group_id},
        )
