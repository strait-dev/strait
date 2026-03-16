use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct LogDrainsService {
    client: Arc<StraitClient>,
}

impl LogDrainsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/log-drains", query, None, None)
            .await
    }

    pub async fn create(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/log-drains", None, None, Some(body))
            .await
    }

    pub async fn get(&self, drain_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/log-drains/{drainID}", &[("drainID", drain_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn update(
        &self,
        drain_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/log-drains/{drainID}", &[("drainID", drain_id)]);
        self.client
            .do_request("PATCH", &path, None, None, Some(body))
            .await
    }

    pub async fn delete(&self, drain_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/log-drains/{drainID}", &[("drainID", drain_id)]);
        self.client
            .do_request_no_content("DELETE", &path, None, None, None)
            .await
    }
}
