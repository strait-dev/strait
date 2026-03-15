use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct WorkflowRunsService {
    client: Arc<StraitClient>,
}

impl WorkflowRunsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(&self, query: Option<&[(&str, &str)]>) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("GET", "/v1/workflow-runs", query, None, None).await
    }

    pub async fn get(&self, workflow_run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/workflow-runs/{workflowRunID}", &[("workflowRunID", workflow_run_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn delete(&self, workflow_run_id: &str) -> Result<(), StraitError> {
        let path = substitute_path_params("/v1/workflow-runs/{workflowRunID}", &[("workflowRunID", workflow_run_id)]);
        self.client.do_request_no_content("DELETE", &path, None, None, None).await
    }

    pub async fn pause(&self, workflow_run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/workflow-runs/{workflowRunID}/pause", &[("workflowRunID", workflow_run_id)]);
        self.client.do_request("POST", &path, None, None, None).await
    }

    pub async fn resume(&self, workflow_run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/workflow-runs/{workflowRunID}/resume", &[("workflowRunID", workflow_run_id)]);
        self.client.do_request("POST", &path, None, None, None).await
    }

    pub async fn retry(&self, workflow_run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/workflow-runs/{workflowRunID}/retry", &[("workflowRunID", workflow_run_id)]);
        self.client.do_request("POST", &path, None, None, None).await
    }

    pub async fn list_steps(&self, workflow_run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/workflow-runs/{workflowRunID}/steps", &[("workflowRunID", workflow_run_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn approve_step(&self, workflow_run_id: &str, step_ref: &str, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/approve", &[("workflowRunID", workflow_run_id), ("stepRef", step_ref)]);
        self.client.do_request("POST", &path, None, None, Some(body)).await
    }

    pub async fn retry_step(&self, workflow_run_id: &str, step_ref: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/retry", &[("workflowRunID", workflow_run_id), ("stepRef", step_ref)]);
        self.client.do_request("POST", &path, None, None, None).await
    }

    pub async fn skip_step(&self, workflow_run_id: &str, step_ref: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/skip", &[("workflowRunID", workflow_run_id), ("stepRef", step_ref)]);
        self.client.do_request("POST", &path, None, None, None).await
    }

    pub async fn force_complete_step(&self, workflow_run_id: &str, step_ref: &str, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/force-complete", &[("workflowRunID", workflow_run_id), ("stepRef", step_ref)]);
        self.client.do_request("POST", &path, None, None, Some(body)).await
    }

    pub async fn replay_subtree_step(&self, workflow_run_id: &str, step_ref: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params("/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/replay-subtree", &[("workflowRunID", workflow_run_id), ("stepRef", step_ref)]);
        self.client.do_request("POST", &path, None, None, None).await
    }

    pub async fn bulk_cancel(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("POST", "/v1/workflow-runs/bulk-cancel", None, None, Some(body)).await
    }

    pub async fn bulk_replay(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client.do_request("POST", "/v1/workflow-runs/bulk-replay", None, None, Some(body)).await
    }
}
