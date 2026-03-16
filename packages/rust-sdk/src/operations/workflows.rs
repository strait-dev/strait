use crate::client::StraitClient;
use crate::errors::StraitError;
use crate::http::substitute_path_params;
use std::sync::Arc;

pub struct WorkflowsService {
    client: Arc<StraitClient>,
}

impl WorkflowsService {
    pub fn new(client: Arc<StraitClient>) -> Self {
        Self { client }
    }

    pub async fn list(
        &self,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("GET", "/v1/workflows", query, None, None)
            .await
    }

    pub async fn create(&self, body: serde_json::Value) -> Result<serde_json::Value, StraitError> {
        self.client
            .do_request("POST", "/v1/workflows", None, None, Some(body))
            .await
    }

    pub async fn get(&self, workflow_id: &str) -> Result<serde_json::Value, StraitError> {
        let path =
            substitute_path_params("/v1/workflows/{workflowID}", &[("workflowID", workflow_id)]);
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn update(
        &self,
        workflow_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path =
            substitute_path_params("/v1/workflows/{workflowID}", &[("workflowID", workflow_id)]);
        self.client
            .do_request("PATCH", &path, None, None, Some(body))
            .await
    }

    pub async fn delete(&self, workflow_id: &str) -> Result<(), StraitError> {
        let path =
            substitute_path_params("/v1/workflows/{workflowID}", &[("workflowID", workflow_id)]);
        self.client
            .do_request_no_content("DELETE", &path, None, None, None)
            .await
    }

    pub async fn clone(
        &self,
        workflow_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflows/{workflowID}/clone",
            &[("workflowID", workflow_id)],
        );
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn dry_run(
        &self,
        workflow_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflows/{workflowID}/dry-run",
            &[("workflowID", workflow_id)],
        );
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn plan(
        &self,
        workflow_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflows/{workflowID}/plan",
            &[("workflowID", workflow_id)],
        );
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn simulate(
        &self,
        workflow_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflows/{workflowID}/simulate",
            &[("workflowID", workflow_id)],
        );
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn trigger(
        &self,
        workflow_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflows/{workflowID}/trigger",
            &[("workflowID", workflow_id)],
        );
        self.client
            .do_request("POST", &path, None, None, Some(body))
            .await
    }

    pub async fn get_graph(&self, workflow_run_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflow-runs/{workflowRunID}/graph",
            &[("workflowRunID", workflow_run_id)],
        );
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn get_graph_by_workflow_id(
        &self,
        workflow_id: &str,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflows/{workflowID}/graph",
            &[("workflowID", workflow_id)],
        );
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn list_runs(
        &self,
        workflow_id: &str,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflows/{workflowID}/runs",
            &[("workflowID", workflow_id)],
        );
        self.client
            .do_request("GET", &path, query, None, None)
            .await
    }

    pub async fn list_versions(
        &self,
        workflow_id: &str,
        query: Option<&[(&str, &str)]>,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflows/{workflowID}/versions",
            &[("workflowID", workflow_id)],
        );
        self.client
            .do_request("GET", &path, query, None, None)
            .await
    }

    pub async fn get_version(
        &self,
        workflow_id: &str,
        version_id: &str,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflows/{workflowID}/versions/{versionID}",
            &[("workflowID", workflow_id), ("versionID", version_id)],
        );
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn get_diff(
        &self,
        workflow_id: &str,
        from_version_id: &str,
        to_version_id: &str,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflows/{workflowID}/versions/{fromVersionID}/diff/{toVersionID}",
            &[
                ("workflowID", workflow_id),
                ("fromVersionID", from_version_id),
                ("toVersionID", to_version_id),
            ],
        );
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn get_policy(&self, project_id: &str) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/projects/{projectID}/workflow-policy",
            &[("projectID", project_id)],
        );
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn upsert_policy(
        &self,
        project_id: &str,
        body: serde_json::Value,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/projects/{projectID}/workflow-policy",
            &[("projectID", project_id)],
        );
        self.client
            .do_request("PUT", &path, None, None, Some(body))
            .await
    }

    pub async fn get_explain(
        &self,
        workflow_run_id: &str,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflow-runs/{workflowRunID}/explain",
            &[("workflowRunID", workflow_run_id)],
        );
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn list_labels(
        &self,
        workflow_run_id: &str,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflow-runs/{workflowRunID}/labels",
            &[("workflowRunID", workflow_run_id)],
        );
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn get_impact(
        &self,
        workflow_id: &str,
        version_id: &str,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflows/{workflowID}/versions/{versionID}/impact",
            &[("workflowID", workflow_id), ("versionID", version_id)],
        );
        self.client.do_request("GET", &path, None, None, None).await
    }

    pub async fn list_steps_by_version(
        &self,
        workflow_id: &str,
        version_id: &str,
    ) -> Result<serde_json::Value, StraitError> {
        let path = substitute_path_params(
            "/v1/workflows/{workflowID}/versions/{versionID}/steps",
            &[("workflowID", workflow_id), ("versionID", version_id)],
        );
        self.client.do_request("GET", &path, None, None, None).await
    }
}
