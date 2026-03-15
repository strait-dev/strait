use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct SecretsService {
    client: Arc<StraitClient>,
}

impl SecretsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/secrets", query, None, None)
            .await
    }

    pub async fn create(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/secrets", None, None, Some(body))
            .await
    }

    pub async fn delete(&self, secret_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/secrets/{secretID}", &[("secretID", secret_id)]);
        self.client
            .do_request_no_content("DELETE", &path, None, None, None)
            .await
    }
}
