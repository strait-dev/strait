"""Job management operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class JobsService(BaseService):
    def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/jobs", query=query)

    def create(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/jobs", body=body)

    def get(self, job_id: str) -> dict[str, Any]:
        return self._request("GET", "/v1/jobs/{jobID}", path_params={"jobID": job_id})

    def update(self, job_id: str, body: Any) -> dict[str, Any]:
        return self._request("PATCH", "/v1/jobs/{jobID}", path_params={"jobID": job_id}, body=body)

    def delete(self, job_id: str) -> dict[str, Any]:
        return self._request("DELETE", "/v1/jobs/{jobID}", path_params={"jobID": job_id})

    def clone(self, job_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/jobs/{jobID}/clone", path_params={"jobID": job_id}, body=body,
        )

    def get_health(self, job_id: str) -> dict[str, Any]:
        return self._request("GET", "/v1/jobs/{jobID}/health", path_params={"jobID": job_id})

    def trigger(self, job_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/jobs/{jobID}/trigger", path_params={"jobID": job_id}, body=body,
        )

    def bulk_trigger(self, job_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/jobs/{jobID}/trigger/bulk", path_params={"jobID": job_id}, body=body,
        )

    def list_versions(
        self, job_id: str, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/jobs/{jobID}/versions", path_params={"jobID": job_id}, query=query,
        )

    def get_version(self, job_id: str, version_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/jobs/{jobID}/versions/{versionID}",
            path_params={"jobID": job_id, "versionID": version_id},
        )

    def list_dependencies(self, job_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/jobs/{jobID}/dependencies", path_params={"jobID": job_id},
        )

    def create_dependency(self, job_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "POST", "/v1/jobs/{jobID}/dependencies", path_params={"jobID": job_id}, body=body,
        )

    def delete_dependency(self, job_id: str, dep_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/jobs/{jobID}/dependencies/{depID}",
            path_params={"jobID": job_id, "depID": dep_id},
        )

    def batch(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/jobs/batch", body=body)

    def batch_disable(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/jobs/batch-disable", body=body)

    def batch_enable(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/jobs/batch-enable", body=body)


class AsyncJobsService(AsyncBaseService):
    async def list(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/jobs", query=query)

    async def create(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/jobs", body=body)

    async def get(self, job_id: str) -> dict[str, Any]:
        return await self._request("GET", "/v1/jobs/{jobID}", path_params={"jobID": job_id})

    async def update(self, job_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "PATCH", "/v1/jobs/{jobID}", path_params={"jobID": job_id}, body=body,
        )

    async def delete(self, job_id: str) -> dict[str, Any]:
        return await self._request("DELETE", "/v1/jobs/{jobID}", path_params={"jobID": job_id})

    async def clone(self, job_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/jobs/{jobID}/clone", path_params={"jobID": job_id}, body=body,
        )

    async def get_health(self, job_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/jobs/{jobID}/health", path_params={"jobID": job_id},
        )

    async def trigger(self, job_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/jobs/{jobID}/trigger", path_params={"jobID": job_id}, body=body,
        )

    async def bulk_trigger(self, job_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/jobs/{jobID}/trigger/bulk", path_params={"jobID": job_id}, body=body,
        )

    async def list_versions(
        self, job_id: str, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/jobs/{jobID}/versions", path_params={"jobID": job_id}, query=query,
        )

    async def get_version(self, job_id: str, version_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/jobs/{jobID}/versions/{versionID}",
            path_params={"jobID": job_id, "versionID": version_id},
        )

    async def list_dependencies(self, job_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/jobs/{jobID}/dependencies", path_params={"jobID": job_id},
        )

    async def create_dependency(self, job_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "POST", "/v1/jobs/{jobID}/dependencies", path_params={"jobID": job_id}, body=body,
        )

    async def delete_dependency(self, job_id: str, dep_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/jobs/{jobID}/dependencies/{depID}",
            path_params={"jobID": job_id, "depID": dep_id},
        )

    async def batch(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/jobs/batch", body=body)

    async def batch_disable(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/jobs/batch-disable", body=body)

    async def batch_enable(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/jobs/batch-enable", body=body)
