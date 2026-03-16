use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct JobsService {
    client: Arc<StraitClient>,
}

impl JobsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/jobs", query, None, None)
            .await
    }

    pub async fn create(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/jobs", None, None, Some(body))
            .await
    }

    pub async fn get(&self, job_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/jobs/{jobID}", &[("jobID", job_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn update(
        &self,
        job_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/jobs/{jobID}", &[("jobID", job_id)]);
        self.client
            .do_request("PATCH", &path, None, None, Some(body))
            .await
    }

    pub async fn delete(&self, job_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/jobs/{jobID}", &[("jobID", job_id)]);
        self.client
            .do_request_no_content("DELETE", &path, None, None, None)
            .await
    }

    pub async fn clone(
        &self,
        job_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/jobs/{jobID}/clone", &[("jobID", job_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn get_health(&self, job_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/jobs/{jobID}/health", &[("jobID", job_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn trigger(
        &self,
        job_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/jobs/{jobID}/trigger", &[("jobID", job_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn bulk_trigger(
        &self,
        job_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/jobs/{jobID}/trigger/bulk", &[("jobID", job_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn list_versions(
        &self,
        job_id: &str,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/jobs/{jobID}/versions", &[("jobID", job_id)]);
        self.client
            .do_request("GET", &path, query, None, None)
            .await
    }

    pub async fn get_version(
        &self,
        job_id: &str,
        version_id: &str,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/jobs/{jobID}/versions/{versionID}",
            &[("jobID", job_id), ("versionID", version_id)],
        );
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn list_dependencies(&self, job_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/jobs/{jobID}/dependencies", &[("jobID", job_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn create_dependency(
        &self,
        job_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/jobs/{jobID}/dependencies", &[("jobID", job_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn delete_dependency(&self, job_id: &str, dep_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params(
            "/v1/jobs/{jobID}/dependencies/{depID}",
            &[("jobID", job_id), ("depID", dep_id)],
        );
        self.client
            .do_request_no_content("DELETE", &path, None, None, None)
            .await
    }

    pub async fn batch(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/jobs/batch", None, None, Some(body))
            .await
    }

    pub async fn batch_disable(
        &self,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/jobs/batch-disable", None, None, Some(body))
            .await
    }

    pub async fn batch_enable(
        &self,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/jobs/batch-enable", None, None, Some(body))
            .await
    }
}
