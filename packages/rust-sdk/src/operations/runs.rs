use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct RunsService {
    client: Arc<StraitClient>,
}

impl RunsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/runs", query, None, None)
            .await
    }

    pub async fn get(&self, run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}", &[("runID", run_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn delete(&self, run_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}", &[("runID", run_id)]);
        self.client
            .do_request_no_content("DELETE", &path, None, None, None)
            .await
    }

    pub async fn list_checkpoints(&self, run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/checkpoints", &[("runID", run_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn get_children(
        &self,
        run_id: &str,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/children", &[("runID", run_id)]);
        self.client
            .do_request("GET", &path, query, None, None)
            .await
    }

    pub async fn debug(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/debug", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn get_debug_bundle(&self, run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/debug-bundle", &[("runID", run_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn list_dependency_status(
        &self,
        run_id: &str,
    ) -> Result<serde_json::Value, StraitError> {
        let path =
            substitute_path_params("/v1/runs/{runID}/dependency-status", &[("runID", run_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn dlq_replay(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/dlq-replay", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn list_events(
        &self,
        run_id: &str,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/events", &[("runID", run_id)]);
        self.client
            .do_request("GET", &path, query, None, None)
            .await
    }

    pub async fn delete_idempotency_key(&self, run_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/idempotency-key", &[("runID", run_id)]);
        self.client
            .do_request_no_content("DELETE", &path, None, None, None)
            .await
    }

    pub async fn get_lineage(&self, run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/lineage", &[("runID", run_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn list_outputs(&self, run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/outputs", &[("runID", run_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn replay(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/replay", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn reschedule(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/reschedule", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn list_tool_calls(&self, run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/tool-calls", &[("runID", run_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn get_usage(&self, run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/runs/{runID}/usage", &[("runID", run_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn bulk_cancel(
        &self,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/runs/bulk-cancel", None, None, Some(body))
            .await
    }

    pub async fn bulk_cancel_all(
        &self,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/runs/bulk-cancel-all", None, None, Some(body))
            .await
    }

    pub async fn bulk_dlq_replay(
        &self,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/runs/bulk-dlq-replay", None, None, Some(body))
            .await
    }

    pub async fn bulk_replay(
        &self,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/runs/bulk-replay", None, None, Some(body))
            .await
    }

    pub async fn get_dlq(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/runs/dlq", query, None, None)
            .await
    }
}
