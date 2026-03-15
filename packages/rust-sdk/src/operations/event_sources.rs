use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct EventSourcesService {
    client: Arc<StraitClient>,
}

impl EventSourcesService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(&self, query: Option<&[(&str, &str)]>) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/event-sources", query, None, None).await
    }

    pub async fn create(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("POST", "/v1/event-sources", None, None, Some(body)).await
    }

    pub async fn get(&self, source_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/event-sources/{sourceID}", &[("sourceID", source_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn update(&self, source_id: &str, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/event-sources/{sourceID}", &[("sourceID", source_id)]);
        self.client.do_request("PATCH", &path, None, None, Some(body)).await
    }

    pub async fn delete(&self, source_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/event-sources/{sourceID}", &[("sourceID", source_id)]);
        self.client.do_request_no_content("DELETE", &path, None, None, None).await
    }

    pub async fn subscribe(&self, source_id: &str, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/event-sources/{sourceID}/subscribe", &[("sourceID", source_id)]);
        self.client.do_request("POST", &path, None, None, Some(body)).await
    }

    pub async fn list_subscriptions(&self, source_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/event-sources/{sourceID}/subscriptions", &[("sourceID", source_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn delete_subscription(&self, source_id: &str, sub_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/event-sources/{sourceID}/subscriptions/{subID}", &[("sourceID", source_id), ("subID", sub_id)]);
        self.client.do_request_no_content("DELETE", &path, None, None, None).await
    }

    pub async fn dispatch_event(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("POST", "/v1/event-sources/dispatch", None, None, Some(body)).await
    }
}
