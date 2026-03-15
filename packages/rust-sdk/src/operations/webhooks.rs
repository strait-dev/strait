use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct WebhooksService {
    client: Arc<StraitClient>,
}

impl WebhooksService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list_subscriptions(&self, query: Option<&[(&str, &str)]>) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/webhooks/subscriptions", query, None, None).await
    }

    pub async fn create_subscription(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("POST", "/v1/webhooks/subscriptions", None, None, Some(body)).await
    }

    pub async fn delete_subscription(&self, id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/webhooks/subscriptions/{id}", &[("id", id)]);
        self.client.do_request_no_content("DELETE", &path, None, None, None).await
    }

    pub async fn list_deliveries(&self, query: Option<&[(&str, &str)]>) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/webhooks/deliveries", query, None, None).await
    }

    pub async fn get_delivery(&self, id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/webhooks/deliveries/{id}", &[("id", id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn retry_delivery(&self, id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/webhooks/deliveries/{id}/retry", &[("id", id)]);
        self.client.do_request("POST", &path, None, None, None).await
    }
}
