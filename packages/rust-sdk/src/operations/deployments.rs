use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct DeploymentsService {
    client: Arc<StraitClient>,
}

impl DeploymentsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(&self, query: Option<&[(&str, &str)]>) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/deployments", query, None, None).await
    }

    pub async fn create(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("POST", "/v1/deployments", None, None, Some(body)).await
    }

    pub async fn finalize(&self, deployment_id: &str, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/deployments/{deploymentID}/finalize", &[("deploymentID", deployment_id)]);
        self.client.do_request("POST", &path, None, None, Some(body)).await
    }

    pub async fn promote(&self, deployment_id: &str, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/deployments/{deploymentID}/promote", &[("deploymentID", deployment_id)]);
        self.client.do_request("POST", &path, None, None, Some(body)).await
    }

    pub async fn rollback(&self, deployment_id: &str, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/deployments/{deploymentID}/rollback", &[("deploymentID", deployment_id)]);
        self.client.do_request("POST", &path, None, None, Some(body)).await
    }
}
