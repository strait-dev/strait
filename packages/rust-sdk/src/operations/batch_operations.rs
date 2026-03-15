use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct BatchOperationsService {
    client: Arc<StraitClient>,
}

impl BatchOperationsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/batch-operations", query, None, None)
            .await
    }

    pub async fn get(&self, batch_id: &str) -> Result<serde_json::Value, StraitError> {
        let path =
            substitute_path_params("/v1/batch-operations/{batchID}", &[("batchID", batch_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }
}
