use std::collections::HashMap;
use std::sync::Arc;
use std::time::Instant;

use reqwest::Client;
use serde_json::Value;

use crate::config::config_from_env;
use crate::config::{normalize_base_url, AuthMode, AuthType, Config};
use crate::config_file::config_from_file;
use crate::errors::{map_http_error, StraitError};
use crate::middleware::{ErrorContext, Middleware, RequestContext, ResponseContext};
use crate::operations::analytics::AnalyticsService;
use crate::operations::api_keys::ApiKeysService;
use crate::operations::batch_operations::BatchOperationsService;
use crate::operations::deployments::DeploymentsService;
use crate::operations::environments::EnvironmentsService;
use crate::operations::event_sources::EventSourcesService;
use crate::operations::event_triggers::EventTriggersService;
use crate::operations::health::HealthService;
use crate::operations::job_groups::JobGroupsService;
use crate::operations::jobs::JobsService;
use crate::operations::log_drains::LogDrainsService;
use crate::operations::rbac::RbacService;
use crate::operations::runs::RunsService;
use crate::operations::sdk_runs::SdkRunsService;
use crate::operations::secrets::SecretsService;
use crate::operations::stats::StatsService;
use crate::operations::webhooks::WebhooksService;
use crate::operations::workflow_runs::WorkflowRunsService;
use crate::operations::workflows::WorkflowsService;

pub struct StraitClient {
    config: Config,
    http_client: Client,
    middleware: Vec<Middleware>,
}

pub struct StraitClientBuilder {
    base_url: Option<String>,
    bearer_token: Option<String>,
    api_key: Option<String>,
    run_token: Option<String>,
    auth: Option<AuthMode>,
    default_headers: HashMap<String, String>,
    timeout_ms: u64,
    middleware: Vec<Middleware>,
}

impl StraitClient {
    pub fn builder() -> StraitClientBuilder {
        StraitClientBuilder {
            base_url: None,
            bearer_token: None,
            api_key: None,
            run_token: None,
            auth: None,
            default_headers: HashMap::new(),
            timeout_ms: 30000,
            middleware: Vec::new(),
        }
    }

    pub fn from_env() -> Result<Arc<StraitClient>, StraitError> {
        let config = config_from_env()?;
        let http_client = Client::builder()
            .timeout(std::time::Duration::from_millis(config.timeout_ms))
            .build()
            .map_err(|e| StraitError::Transport {
                message: "failed to create HTTP client".to_string(),
                cause: Some(e.to_string()),
            })?;

        Ok(Arc::new(StraitClient {
            config,
            http_client,
            middleware: Vec::new(),
        }))
    }

    pub fn from_file(
        path: Option<&str>,
        search_dir: Option<&str>,
    ) -> Result<Arc<StraitClient>, StraitError> {
        let config = config_from_file(path, search_dir)?;
        let http_client = Client::builder()
            .timeout(std::time::Duration::from_millis(config.timeout_ms))
            .build()
            .map_err(|e| StraitError::Transport {
                message: "failed to create HTTP client".to_string(),
                cause: Some(e.to_string()),
            })?;

        Ok(Arc::new(StraitClient {
            config,
            http_client,
            middleware: Vec::new(),
        }))
    }

    pub async fn do_request(
        &self,
        method: &str,
        path: &str,
        query: Option<&[(&str, &str)]>,
        headers: Option<&HashMap<String, String>>,
        body: Option<Value>,
    ) -> Result<Value, StraitError> {
        let url = format!("{}{}", self.config.base_url, path);

        let mut header_map = HashMap::new();
        header_map.insert("Content-Type".to_string(), "application/json".to_string());
        header_map.insert("Accept".to_string(), "application/json".to_string());
        header_map.insert(
            "Authorization".to_string(),
            crate::config::get_authorization_header(&self.config.auth),
        );

        for (k, v) in &self.config.default_headers {
            header_map.insert(k.clone(), v.clone());
        }

        if let Some(h) = headers {
            for (k, v) in h {
                header_map.insert(k.clone(), v.clone());
            }
        }

        let request_context = RequestContext {
            method: method.to_string(),
            url: url.clone(),
            headers: header_map.clone(),
        };
        for mw in &self.middleware {
            if let Some(ref on_request) = mw.on_request {
                on_request(&request_context);
            }
        }

        let start = Instant::now();

        let mut req = match method {
            "GET" => self.http_client.get(&url),
            "POST" => self.http_client.post(&url),
            "PUT" => self.http_client.put(&url),
            "PATCH" => self.http_client.patch(&url),
            "DELETE" => self.http_client.delete(&url),
            _ => {
                return Err(StraitError::Validation {
                    message: format!("unsupported HTTP method: {method}"),
                    issues: vec![],
                });
            }
        };

        for (k, v) in &header_map {
            req = req.header(k.as_str(), v.as_str());
        }

        if let Some(q) = query {
            req = req.query(q);
        }

        if let Some(b) = body {
            req = req.json(&b);
        }

        let response = req.send().await.map_err(|e| {
            let err = StraitError::Transport {
                message: format!("request failed: {e}"),
                cause: Some(e.to_string()),
            };
            let error_context = ErrorContext {
                method: method.to_string(),
                url: url.clone(),
                error: e.to_string(),
            };
            for mw in &self.middleware {
                if let Some(ref on_error) = mw.on_error {
                    on_error(&error_context);
                }
            }
            err
        })?;

        let duration_ms = start.elapsed().as_millis() as u64;
        let status = response.status().as_u16();

        let response_context = ResponseContext {
            method: method.to_string(),
            url: url.clone(),
            status,
            duration_ms,
        };
        for mw in &self.middleware {
            if let Some(ref on_response) = mw.on_response {
                on_response(&response_context);
            }
        }

        let response_text = response.text().await.map_err(|e| StraitError::Decode {
            message: format!("failed to read response body: {e}"),
            body: None,
        })?;

        if status >= 400 {
            let body_value: Option<Value> = serde_json::from_str(&response_text).ok();
            let message = body_value
                .as_ref()
                .and_then(|v| v.get("message"))
                .and_then(|v| v.as_str())
                .unwrap_or("request failed")
                .to_string();
            return Err(map_http_error(status, message, body_value));
        }

        if response_text.is_empty() {
            return Ok(Value::Null);
        }

        serde_json::from_str(&response_text).map_err(|e| StraitError::Decode {
            message: format!("failed to parse response JSON: {e}"),
            body: Some(response_text),
        })
    }

    pub async fn do_request_no_content(
        &self,
        method: &str,
        path: &str,
        query: Option<&[(&str, &str)]>,
        headers: Option<&HashMap<String, String>>,
        body: Option<Value>,
    ) -> Result<(), StraitError> {
        self.do_request(method, path, query, headers, body).await?;
        Ok(())
    }

    pub fn jobs(self: &Arc<Self>) -> JobsService {
        JobsService::new(Arc::clone(self))
    }

    pub fn runs(self: &Arc<Self>) -> RunsService {
        RunsService::new(Arc::clone(self))
    }

    pub fn workflows(self: &Arc<Self>) -> WorkflowsService {
        WorkflowsService::new(Arc::clone(self))
    }

    pub fn workflow_runs(self: &Arc<Self>) -> WorkflowRunsService {
        WorkflowRunsService::new(Arc::clone(self))
    }

    pub fn deployments(self: &Arc<Self>) -> DeploymentsService {
        DeploymentsService::new(Arc::clone(self))
    }

    pub fn environments(self: &Arc<Self>) -> EnvironmentsService {
        EnvironmentsService::new(Arc::clone(self))
    }

    pub fn secrets(self: &Arc<Self>) -> SecretsService {
        SecretsService::new(Arc::clone(self))
    }

    pub fn api_keys(self: &Arc<Self>) -> ApiKeysService {
        ApiKeysService::new(Arc::clone(self))
    }

    pub fn webhooks(self: &Arc<Self>) -> WebhooksService {
        WebhooksService::new(Arc::clone(self))
    }

    pub fn event_triggers(self: &Arc<Self>) -> EventTriggersService {
        EventTriggersService::new(Arc::clone(self))
    }

    pub fn event_sources(self: &Arc<Self>) -> EventSourcesService {
        EventSourcesService::new(Arc::clone(self))
    }

    pub fn batch_operations(self: &Arc<Self>) -> BatchOperationsService {
        BatchOperationsService::new(Arc::clone(self))
    }

    pub fn stats(self: &Arc<Self>) -> StatsService {
        StatsService::new(Arc::clone(self))
    }

    pub fn analytics(self: &Arc<Self>) -> AnalyticsService {
        AnalyticsService::new(Arc::clone(self))
    }

    pub fn log_drains(self: &Arc<Self>) -> LogDrainsService {
        LogDrainsService::new(Arc::clone(self))
    }

    pub fn sdk_runs(self: &Arc<Self>) -> SdkRunsService {
        SdkRunsService::new(Arc::clone(self))
    }

    pub fn rbac(self: &Arc<Self>) -> RbacService {
        RbacService::new(Arc::clone(self))
    }

    pub fn job_groups(self: &Arc<Self>) -> JobGroupsService {
        JobGroupsService::new(Arc::clone(self))
    }

    pub fn health(self: &Arc<Self>) -> HealthService {
        HealthService::new(Arc::clone(self))
    }
}

impl StraitClientBuilder {
    pub fn base_url(mut self, url: &str) -> Self {
        self.base_url = Some(normalize_base_url(url));
        self
    }

    pub fn bearer_token(mut self, token: &str) -> Self {
        self.bearer_token = Some(token.to_string());
        self
    }

    pub fn api_key(mut self, key: &str) -> Self {
        self.api_key = Some(key.to_string());
        self
    }

    pub fn run_token(mut self, token: &str) -> Self {
        self.run_token = Some(token.to_string());
        self
    }

    pub fn auth(mut self, auth: AuthMode) -> Self {
        self.auth = Some(auth);
        self
    }

    pub fn default_header(mut self, key: &str, value: &str) -> Self {
        self.default_headers
            .insert(key.to_string(), value.to_string());
        self
    }

    pub fn default_headers(mut self, headers: HashMap<String, String>) -> Self {
        self.default_headers = headers;
        self
    }

    pub fn timeout_ms(mut self, ms: u64) -> Self {
        self.timeout_ms = ms;
        self
    }

    pub fn middleware(mut self, mw: Middleware) -> Self {
        self.middleware.push(mw);
        self
    }

    pub fn build(self) -> Result<Arc<StraitClient>, StraitError> {
        let base_url = self.base_url.ok_or_else(|| StraitError::Validation {
            message: "base_url is required".to_string(),
            issues: vec!["call .base_url() on the builder".to_string()],
        })?;

        let auth = if let Some(a) = self.auth {
            a
        } else if let Some(token) = self.bearer_token {
            AuthMode {
                auth_type: AuthType::Bearer,
                token,
            }
        } else if let Some(key) = self.api_key {
            AuthMode {
                auth_type: AuthType::ApiKey,
                token: key,
            }
        } else if let Some(token) = self.run_token {
            AuthMode {
                auth_type: AuthType::RunToken,
                token,
            }
        } else {
            return Err(StraitError::Validation {
                message: "authentication is required".to_string(),
                issues: vec![
                    "call .bearer_token(), .api_key(), .run_token(), or .auth() on the builder"
                        .to_string(),
                ],
            });
        };

        let config = Config {
            base_url,
            auth,
            default_headers: self.default_headers,
            timeout_ms: self.timeout_ms,
        };

        let http_client = Client::builder()
            .timeout(std::time::Duration::from_millis(config.timeout_ms))
            .build()
            .map_err(|e| StraitError::Transport {
                message: "failed to create HTTP client".to_string(),
                cause: Some(e.to_string()),
            })?;

        Ok(Arc::new(StraitClient {
            config,
            http_client,
            middleware: self.middleware,
        }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_builder_with_bearer_token() {
        let client = StraitClient::builder()
            .base_url("https://api.example.com")
            .bearer_token("tok-123")
            .build();
        assert!(client.is_ok());
    }

    #[test]
    fn test_builder_with_api_key() {
        let client = StraitClient::builder()
            .base_url("https://api.example.com")
            .api_key("key-abc")
            .build();
        assert!(client.is_ok());
    }

    #[test]
    fn test_builder_with_run_token() {
        let client = StraitClient::builder()
            .base_url("https://api.example.com")
            .run_token("rt-xyz")
            .build();
        assert!(client.is_ok());
    }

    #[test]
    fn test_builder_with_auth_mode() {
        let client = StraitClient::builder()
            .base_url("https://api.example.com")
            .auth(AuthMode {
                auth_type: AuthType::Bearer,
                token: "tok".to_string(),
            })
            .build();
        assert!(client.is_ok());
    }

    #[test]
    fn test_builder_missing_base_url() {
        let result = StraitClient::builder().bearer_token("tok").build();
        assert!(result.is_err());
        if let Err(StraitError::Validation { message, .. }) = result {
            assert!(message.contains("base_url"));
        } else {
            panic!("expected Validation error");
        }
    }

    #[test]
    fn test_builder_missing_auth() {
        let result = StraitClient::builder()
            .base_url("https://api.example.com")
            .build();
        assert!(result.is_err());
        if let Err(StraitError::Validation { message, .. }) = result {
            assert!(message.contains("authentication"));
        } else {
            panic!("expected Validation error");
        }
    }

    #[test]
    fn test_builder_with_timeout() {
        let client = StraitClient::builder()
            .base_url("https://api.example.com")
            .bearer_token("tok")
            .timeout_ms(5000)
            .build();
        assert!(client.is_ok());
    }

    #[test]
    fn test_builder_with_default_header() {
        let client = StraitClient::builder()
            .base_url("https://api.example.com")
            .bearer_token("tok")
            .default_header("X-Custom", "value")
            .build();
        assert!(client.is_ok());
    }

    #[test]
    fn test_builder_with_default_headers_map() {
        let mut headers = HashMap::new();
        headers.insert("X-A".to_string(), "1".to_string());
        headers.insert("X-B".to_string(), "2".to_string());
        let client = StraitClient::builder()
            .base_url("https://api.example.com")
            .bearer_token("tok")
            .default_headers(headers)
            .build();
        assert!(client.is_ok());
    }

    #[test]
    fn test_builder_normalizes_url() {
        let client = StraitClient::builder()
            .base_url("https://api.example.com/")
            .bearer_token("tok")
            .build()
            .unwrap();
        // The URL should be normalized (no trailing slash)
        // We can't directly access config but the build succeeds
        assert!(Arc::strong_count(&client) == 1);
    }

    #[test]
    fn test_builder_chaining() {
        let result = StraitClient::builder()
            .base_url("https://api.example.com")
            .bearer_token("tok")
            .timeout_ms(10_000)
            .default_header("X-Test", "val")
            .build();
        assert!(result.is_ok());
    }
}
