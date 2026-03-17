use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct SdkRunsService {
    client: Arc<StraitClient>,
}

impl SdkRunsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn annotate_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/annotate", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn checkpoint_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/checkpoint", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn complete_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/complete", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn continue_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/continue", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn fail_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/fail", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn heartbeat_run(&self, run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/heartbeat", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, None)
            .await
    }

    pub async fn log_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/log", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn output_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/output", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn progress_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/progress", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn spawn_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/spawn", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn tool_call_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/tool-call", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn usage_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/usage", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn wait_for_event_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path =
            substitute_path_params("/sdk/v1/runs/{runID}/wait-for-event", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn set_state(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/state", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn list_state(&self, run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/state", &[("runID", run_id)]);
        self.client
            .do_request("GET", &path, None, None, None)
            .await
    }

    pub async fn get_state(
        &self,
        run_id: &str,
        key: &str,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/sdk/v1/runs/{runID}/state/{key}",
            &[("runID", run_id), ("key", key)],
        );
        self.client
            .do_request("GET", &path, None, None, None)
            .await
    }

    pub async fn delete_state(
        &self,
        run_id: &str,
        key: &str,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/sdk/v1/runs/{runID}/state/{key}",
            &[("runID", run_id), ("key", key)],
        );
        self.client
            .do_request("DELETE", &path, None, None, None)
            .await
    }

    pub async fn get_payload(&self, run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/payload", &[("runID", run_id)]);
        self.client
            .do_request("GET", &path, None, None, None)
            .await
    }

    pub async fn resources_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/resources", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn stream_run(
        &self,
        run_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/sdk/v1/runs/{runID}/stream", &[("runID", run_id)]);
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }
}
