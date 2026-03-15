use serde_json::{json, Value};

#[derive(Debug, Clone, Default)]
pub struct JobOptions {
    pub name: Option<String>,
    pub slug: Option<String>,
    pub endpoint_url: Option<String>,
    pub project_id: Option<String>,
    pub description: Option<String>,
    pub group_id: Option<String>,
    pub tags: Option<Vec<String>>,
    pub environment_id: Option<String>,
    pub cron: Option<String>,
    pub timezone: Option<String>,
    pub execution_window_cron: Option<String>,
    pub max_concurrency: Option<u32>,
    pub rate_limit_max: Option<u32>,
    pub rate_limit_window_secs: Option<u32>,
    pub max_attempts: Option<u32>,
    pub retry_strategy: Option<String>,
    pub retry_delays_secs: Option<Vec<u32>>,
    pub timeout_secs: Option<u32>,
    pub run_ttl_secs: Option<u32>,
    pub dedup_window_secs: Option<u32>,
    pub webhook_url: Option<String>,
    pub webhook_secret: Option<String>,
    pub fallback_endpoint_url: Option<String>,
}

#[derive(Debug, Clone, Default)]
pub struct TriggerJobInput {
    pub job_id: Option<String>,
    pub payload: Option<Value>,
    pub idempotency_key: Option<String>,
    pub priority: Option<i32>,
    pub dry_run: Option<bool>,
    pub metadata: Option<std::collections::HashMap<String, String>>,
    pub scheduled_at: Option<String>,
}

#[derive(Debug)]
pub struct JobDefinition {
    pub kind: String,
    pub slug: Option<String>,
    opts: JobOptions,
    pub last_registered_job_id: Option<String>,
}

impl JobDefinition {
    pub fn to_registration_body(&self, project_id: Option<&str>) -> Value {
        let pid = project_id
            .map(|s| s.to_string())
            .or_else(|| self.opts.project_id.clone());
        let mut body = json!({});
        if let Some(v) = &self.opts.name { body["name"] = json!(v); }
        if let Some(v) = &self.opts.slug { body["slug"] = json!(v); }
        if let Some(v) = &self.opts.endpoint_url { body["endpoint_url"] = json!(v); }
        if let Some(v) = &pid { body["project_id"] = json!(v); }
        if let Some(v) = &self.opts.description { body["description"] = json!(v); }
        if let Some(v) = &self.opts.group_id { body["group_id"] = json!(v); }
        if let Some(v) = &self.opts.tags { body["tags"] = json!(v); }
        if let Some(v) = &self.opts.environment_id { body["environment_id"] = json!(v); }
        if let Some(v) = &self.opts.cron { body["cron"] = json!(v); }
        if let Some(v) = &self.opts.timezone { body["timezone"] = json!(v); }
        if let Some(v) = &self.opts.execution_window_cron { body["execution_window_cron"] = json!(v); }
        if let Some(v) = self.opts.max_concurrency { body["max_concurrency"] = json!(v); }
        if let Some(v) = self.opts.rate_limit_max { body["rate_limit_max"] = json!(v); }
        if let Some(v) = self.opts.rate_limit_window_secs { body["rate_limit_window_secs"] = json!(v); }
        if let Some(v) = self.opts.max_attempts { body["max_attempts"] = json!(v); }
        if let Some(v) = &self.opts.retry_strategy { body["retry_strategy"] = json!(v); }
        if let Some(v) = &self.opts.retry_delays_secs { body["retry_delays_secs"] = json!(v); }
        if let Some(v) = self.opts.timeout_secs { body["timeout_secs"] = json!(v); }
        if let Some(v) = self.opts.run_ttl_secs { body["run_ttl_secs"] = json!(v); }
        if let Some(v) = self.opts.dedup_window_secs { body["dedup_window_secs"] = json!(v); }
        if let Some(v) = &self.opts.webhook_url { body["webhook_url"] = json!(v); }
        if let Some(v) = &self.opts.webhook_secret { body["webhook_secret"] = json!(v); }
        if let Some(v) = &self.opts.fallback_endpoint_url { body["fallback_endpoint_url"] = json!(v); }
        body
    }
}

pub fn define_job(opts: JobOptions) -> JobDefinition {
    JobDefinition {
        kind: "job".to_string(),
        slug: opts.slug.clone(),
        opts,
        last_registered_job_id: None,
    }
}
