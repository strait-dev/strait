use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct RbacService {
    client: Arc<StraitClient>,
}

impl RbacService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list_audit_events(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/rbac/audit-events", query, None, None)
            .await
    }

    pub async fn list_members(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/rbac/members", query, None, None)
            .await
    }

    pub async fn create_member(
        &self,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/rbac/members", None, None, Some(body))
            .await
    }

    pub async fn delete_member(&self, user_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/rbac/members/{userID}", &[("userID", user_id)]);
        self.client
            .do_request_no_content("DELETE", &path, None, None, None)
            .await
    }

    pub async fn bulk_member(
        &self,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/rbac/members/bulk", None, None, Some(body))
            .await
    }

    pub async fn list_roles(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/rbac/roles", query, None, None)
            .await
    }

    pub async fn create_role(
        &self,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/rbac/roles", None, None, Some(body))
            .await
    }

    pub async fn get_role(&self, role_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/rbac/roles/{roleID}", &[("roleID", role_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn update_role(
        &self,
        role_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/rbac/roles/{roleID}", &[("roleID", role_id)]);
        self.client
            .do_request("PATCH", &path, None, None, Some(body))
            .await
    }

    pub async fn delete_role(&self, role_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/rbac/roles/{roleID}", &[("roleID", role_id)]);
        self.client
            .do_request_no_content("DELETE", &path, None, None, None)
            .await
    }

    pub async fn list_resource_policies(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/rbac/resource-policies", query, None, None)
            .await
    }

    pub async fn create_resource_policy(
        &self,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/rbac/resource-policies", None, None, Some(body))
            .await
    }

    pub async fn delete_resource_policy(&self, policy_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params(
            "/v1/rbac/resource-policies/{policyID}",
            &[("policyID", policy_id)],
        );
        self.client
            .do_request_no_content("DELETE", &path, None, None, None)
            .await
    }

    pub async fn list_tag_policies(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/rbac/tag-policies", query, None, None)
            .await
    }

    pub async fn create_tag_policy(
        &self,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/rbac/tag-policies", None, None, Some(body))
            .await
    }

    pub async fn delete_tag_policy(&self, policy_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params(
            "/v1/rbac/tag-policies/{policyID}",
            &[("policyID", policy_id)],
        );
        self.client
            .do_request_no_content("DELETE", &path, None, None, None)
            .await
    }

    pub async fn seed_roles(&self) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/rbac/seed", None, None, None)
            .await
    }
}
