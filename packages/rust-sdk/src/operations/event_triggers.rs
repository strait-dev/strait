use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct EventTriggersService {
    client: Arc<StraitClient>,
}

impl EventTriggersService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list_events(&self, query: Option<&[(&str, &str)]>) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/event-triggers/events", query, None, None).await
    }

    pub async fn get_event(&self, event_key: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/event-triggers/events/{eventKey}", &[("eventKey", event_key)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn delete_event(&self, event_key: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/event-triggers/events/{eventKey}", &[("eventKey", event_key)]);
        self.client.do_request_no_content("DELETE", &path, None, None, None).await
    }

    pub async fn send_event(&self, event_key: &str, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/event-triggers/events/{eventKey}/send", &[("eventKey", event_key)]);
        self.client.do_request("POST", &path, None, None, Some(body)).await
    }

    pub async fn send_prefix(&self, prefix: &str, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/event-triggers/prefix/{prefix}/send", &[("prefix", prefix)]);
        self.client.do_request("POST", &path, None, None, Some(body)).await
    }

    pub async fn purge_event(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("POST", "/v1/event-triggers/purge", None, None, Some(body)).await
    }

    pub async fn get_stat(&self) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/event-triggers/stats", None, None, None).await
    }
}
