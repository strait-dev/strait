use serde_json::{json, Value};
use super::steps::Step;
use super::dag_validation::validate_dag;

#[derive(Debug, Clone, Default)]
pub struct WorkflowOptions {
    pub name: Option<String>,
    pub slug: Option<String>,
    pub steps: Option<Vec<Step>>,
    pub project_id: Option<String>,
    pub description: Option<String>,
    pub tags: Option<Vec<String>>,
    pub environment_id: Option<String>,
    pub max_concurrent_runs: Option<u32>,
    pub max_parallel_steps: Option<u32>,
    pub timeout_secs: Option<u32>,
    pub max_attempts: Option<u32>,
    pub retry_strategy: Option<String>,
    pub cron: Option<String>,
    pub timezone: Option<String>,
    pub webhook_url: Option<String>,
    pub webhook_secret: Option<String>,
}

#[derive(Debug, Clone, Default)]
pub struct TriggerWorkflowInput {
    pub workflow_id: Option<String>,
    pub payload: Option<Value>,
    pub idempotency_key: Option<String>,
    pub priority: Option<i32>,
    pub dry_run: Option<bool>,
    pub metadata: Option<std::collections::HashMap<String, String>>,
    pub step_overrides: Option<Value>,
}

#[derive(Debug)]
pub struct WorkflowDefinition {
    pub kind: String,
    pub slug: Option<String>,
    opts: WorkflowOptions,
    pub last_registered_workflow_id: Option<String>,
}

impl WorkflowDefinition {
    pub fn to_registration_body(&self, project_id: Option<&str>) -> Result<Value, crate::errors::StraitError> {
        let pid = project_id
            .map(|s| s.to_string())
            .or_else(|| self.opts.project_id.clone());
        let mut body = json!({});
        if let Some(v) = &self.opts.name { body["name"] = json!(v); }
        if let Some(v) = &self.opts.slug { body["slug"] = json!(v); }
        if let Some(v) = &pid { body["project_id"] = json!(v); }
        if let Some(v) = &self.opts.description { body["description"] = json!(v); }
        if let Some(v) = &self.opts.tags { body["tags"] = json!(v); }
        if let Some(v) = &self.opts.environment_id { body["environment_id"] = json!(v); }
        if let Some(v) = self.opts.max_concurrent_runs { body["max_concurrent_runs"] = json!(v); }
        if let Some(v) = self.opts.max_parallel_steps { body["max_parallel_steps"] = json!(v); }
        if let Some(v) = self.opts.timeout_secs { body["timeout_secs"] = json!(v); }
        if let Some(v) = self.opts.max_attempts { body["max_attempts"] = json!(v); }
        if let Some(v) = &self.opts.retry_strategy { body["retry_strategy"] = json!(v); }
        if let Some(v) = &self.opts.cron { body["cron"] = json!(v); }
        if let Some(v) = &self.opts.timezone { body["timezone"] = json!(v); }
        if let Some(v) = &self.opts.webhook_url { body["webhook_url"] = json!(v); }
        if let Some(v) = &self.opts.webhook_secret { body["webhook_secret"] = json!(v); }

        if let Some(steps) = &self.opts.steps {
            validate_dag(steps)?;
            body["steps"] = json!(steps.iter().map(|s| s.to_api()).collect::<Vec<_>>());
        }

        Ok(body)
    }
}

pub fn define_workflow(opts: WorkflowOptions) -> WorkflowDefinition {
    WorkflowDefinition {
        kind: "workflow".to_string(),
        slug: opts.slug.clone(),
        opts,
        last_registered_workflow_id: None,
    }
}
