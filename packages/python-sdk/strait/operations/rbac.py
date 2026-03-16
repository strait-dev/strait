"""Role-based access control operations."""

from __future__ import annotations

from typing import Any

from strait.operations._base import AsyncBaseService, BaseService


class RBACService(BaseService):
    def list_audit_events(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/audit-events", query=query)

    def list_members(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/members", query=query)

    def create_member(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/members", body=body)

    def delete_member(self, user_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/members/{userID}", path_params={"userID": user_id},
        )

    def bulk_member(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/members/bulk", body=body)

    def list_roles(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/roles", query=query)

    def create_role(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/roles", body=body)

    def get_role(self, role_id: str) -> dict[str, Any]:
        return self._request(
            "GET", "/v1/roles/{roleID}", path_params={"roleID": role_id},
        )

    def update_role(self, role_id: str, body: Any) -> dict[str, Any]:
        return self._request(
            "PATCH", "/v1/roles/{roleID}", path_params={"roleID": role_id}, body=body,
        )

    def delete_role(self, role_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/roles/{roleID}", path_params={"roleID": role_id},
        )

    def list_resource_policies(
        self, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return self._request("GET", "/v1/resource-policies", query=query)

    def create_resource_policy(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/resource-policies", body=body)

    def delete_resource_policy(self, policy_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/resource-policies/{policyID}",
            path_params={"policyID": policy_id},
        )

    def list_tag_policies(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return self._request("GET", "/v1/tag-policies", query=query)

    def create_tag_policy(self, body: Any) -> dict[str, Any]:
        return self._request("POST", "/v1/tag-policies", body=body)

    def delete_tag_policy(self, policy_id: str) -> dict[str, Any]:
        return self._request(
            "DELETE", "/v1/tag-policies/{policyID}", path_params={"policyID": policy_id},
        )

    def seed_roles(self) -> dict[str, Any]:
        return self._request("POST", "/v1/seed-roles")


class AsyncRBACService(AsyncBaseService):
    async def list_audit_events(
        self, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return await self._request("GET", "/v1/audit-events", query=query)

    async def list_members(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/members", query=query)

    async def create_member(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/members", body=body)

    async def delete_member(self, user_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/members/{userID}", path_params={"userID": user_id},
        )

    async def bulk_member(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/members/bulk", body=body)

    async def list_roles(self, *, query: dict[str, str] | None = None) -> dict[str, Any]:
        return await self._request("GET", "/v1/roles", query=query)

    async def create_role(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/roles", body=body)

    async def get_role(self, role_id: str) -> dict[str, Any]:
        return await self._request(
            "GET", "/v1/roles/{roleID}", path_params={"roleID": role_id},
        )

    async def update_role(self, role_id: str, body: Any) -> dict[str, Any]:
        return await self._request(
            "PATCH", "/v1/roles/{roleID}", path_params={"roleID": role_id}, body=body,
        )

    async def delete_role(self, role_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/roles/{roleID}", path_params={"roleID": role_id},
        )

    async def list_resource_policies(
        self, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return await self._request("GET", "/v1/resource-policies", query=query)

    async def create_resource_policy(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/resource-policies", body=body)

    async def delete_resource_policy(self, policy_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/resource-policies/{policyID}",
            path_params={"policyID": policy_id},
        )

    async def list_tag_policies(
        self, *, query: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return await self._request("GET", "/v1/tag-policies", query=query)

    async def create_tag_policy(self, body: Any) -> dict[str, Any]:
        return await self._request("POST", "/v1/tag-policies", body=body)

    async def delete_tag_policy(self, policy_id: str) -> dict[str, Any]:
        return await self._request(
            "DELETE", "/v1/tag-policies/{policyID}", path_params={"policyID": policy_id},
        )

    async def seed_roles(self) -> dict[str, Any]:
        return await self._request("POST", "/v1/seed-roles")
