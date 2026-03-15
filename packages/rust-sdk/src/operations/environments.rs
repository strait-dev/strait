use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct EnvironmentsService {
    client: Arc<StraitClient>,
}

impl EnvironmentsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(&self, query: Option<&[(&str, &str)]>) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/environments", query, None, None).await
    }

    pub async fn create(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("POST", "/v1/environments", None, None, Some(body)).await
    }

    pub async fn get(&self, env_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/environments/{envID}", &[("envID", env_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn update(&self, env_id: &str, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/environments/{envID}", &[("envID", env_id)]);
        self.client.do_request("PATCH", &path, None, None, Some(body)).await
    }

    pub async fn delete(&self, env_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/environments/{envID}", &[("envID", env_id)]);
        self.client.do_request_no_content("DELETE", &path, None, None, None).await
    }

    pub async fn list_variables(&self, env_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/environments/{envID}/variables", &[("envID", env_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }
}
