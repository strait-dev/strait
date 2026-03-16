use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct JobGroupsService {
    client: Arc<StraitClient>,
}

impl JobGroupsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/job-groups", query, None, None)
            .await
    }

    pub async fn create(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/job-groups", None, None, Some(body))
            .await
    }

    pub async fn get(&self, group_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/job-groups/{groupID}", &[("groupID", group_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn update(
        &self,
        group_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/job-groups/{groupID}", &[("groupID", group_id)]);
        self.client
            .do_request("PATCH", &path, None, None, Some(body))
            .await
    }

    pub async fn delete(&self, group_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/job-groups/{groupID}", &[("groupID", group_id)]);
        self.client
            .do_request_no_content("DELETE", &path, None, None, None)
            .await
    }

    pub async fn list_jobs(
        &self,
        group_id: &str,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        let path =
            substitute_path_params("/v1/job-groups/{groupID}/jobs", &[("groupID", group_id)]);
        self.client
            .do_request("GET", &path, query, None, None)
            .await
    }

    pub async fn pause_all(&self, group_id: &str) -> Result<serde_json::Value, StraitError> {
        let path =
            substitute_path_params("/v1/job-groups/{groupID}/pause", &[("groupID", group_id)]);
        self.client
            .do_request("POST", &path, None, None, None)
            .await
    }

    pub async fn resume_all(&self, group_id: &str) -> Result<serde_json::Value, StraitError> {
        let path =
            substitute_path_params("/v1/job-groups/{groupID}/resume", &[("groupID", group_id)]);
        self.client
            .do_request("POST", &path, None, None, None)
            .await
    }

    pub async fn get_stats(&self, group_id: &str) -> Result<serde_json::Value, StraitError> {
        let path =
            substitute_path_params("/v1/job-groups/{groupID}/stats", &[("groupID", group_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }
}
