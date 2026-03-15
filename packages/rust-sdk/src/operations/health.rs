use crate::client::StraitClient;
use crate::errors::StraitError;
use std::sync::Arc;

pub struct HealthService {
    client: Arc<StraitClient>,
}

impl HealthService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(&self) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/health", None, None, None).await
    }

    pub async fn get_ready(&self) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/health/ready", None, None, None).await
    }

    pub async fn list_metrics(&self) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/health/metrics", None, None, None).await
    }
}
