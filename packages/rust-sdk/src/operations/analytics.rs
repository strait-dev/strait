use crate::client::StraitClient;
use crate::errors::StraitError;
use std::sync::Arc;

pub struct AnalyticsService {
    client: Arc<StraitClient>,
}

impl AnalyticsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn get_performance(&self, query: Option<&[(&str, &str)]>) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/analytics/performance", query, None, None).await
    }
}
