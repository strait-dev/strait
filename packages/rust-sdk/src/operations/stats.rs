use crate::client::StraitClient;
use crate::errors::StraitError;
use std::sync::Arc;

pub struct StatsService {
    client: Arc<StraitClient>,
}

impl StatsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(&self, query: Option<&[(&str, &str)]>) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/stats", query, None, None).await
    }
}
