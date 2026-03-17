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
        if let Some(v) = &self.opts.name {
            body["name"] = json!(v);
        }
        if let Some(v) = &self.opts.slug {
            body["slug"] = json!(v);
        }
        if let Some(v) = &self.opts.endpoint_url {
            body["endpoint_url"] = json!(v);
        }
        if let Some(v) = &pid {
            body["project_id"] = json!(v);
        }
        if let Some(v) = &self.opts.description {
            body["description"] = json!(v);
        }
        if let Some(v) = &self.opts.group_id {
            body["group_id"] = json!(v);
        }
        if let Some(v) = &self.opts.tags {
            body["tags"] = json!(v);
        }
        if let Some(v) = &self.opts.environment_id {
            body["environment_id"] = json!(v);
        }
        if let Some(v) = &self.opts.cron {
            body["cron"] = json!(v);
        }
        if let Some(v) = &self.opts.timezone {
            body["timezone"] = json!(v);
        }
        if let Some(v) = &self.opts.execution_window_cron {
            body["execution_window_cron"] = json!(v);
        }
        if let Some(v) = self.opts.max_concurrency {
            body["max_concurrency"] = json!(v);
        }
        if let Some(v) = self.opts.rate_limit_max {
            body["rate_limit_max"] = json!(v);
        }
        if let Some(v) = self.opts.rate_limit_window_secs {
            body["rate_limit_window_secs"] = json!(v);
        }
        if let Some(v) = self.opts.max_attempts {
            body["max_attempts"] = json!(v);
        }
        if let Some(v) = &self.opts.retry_strategy {
            body["retry_strategy"] = json!(v);
        }
        if let Some(v) = &self.opts.retry_delays_secs {
            body["retry_delays_secs"] = json!(v);
        }
        if let Some(v) = self.opts.timeout_secs {
            body["timeout_secs"] = json!(v);
        }
        if let Some(v) = self.opts.run_ttl_secs {
            body["run_ttl_secs"] = json!(v);
        }
        if let Some(v) = self.opts.dedup_window_secs {
            body["dedup_window_secs"] = json!(v);
        }
        if let Some(v) = &self.opts.webhook_url {
            body["webhook_url"] = json!(v);
        }
        if let Some(v) = &self.opts.webhook_secret {
            body["webhook_secret"] = json!(v);
        }
        if let Some(v) = &self.opts.fallback_endpoint_url {
            body["fallback_endpoint_url"] = json!(v);
        }
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_define_job_kind() {
        let job = define_job(JobOptions::default());
        assert_eq!(job.kind, "job");
    }

    #[test]
    fn test_define_job_slug() {
        let job = define_job(JobOptions {
            slug: Some("test-slug".to_string()),
            ..Default::default()
        });
        assert_eq!(job.slug, Some("test-slug".to_string()));
    }

    #[test]
    fn test_define_job_slug_none() {
        let job = define_job(JobOptions::default());
        assert!(job.slug.is_none());
    }

    #[test]
    fn test_define_job_last_registered_none() {
        let job = define_job(JobOptions::default());
        assert!(job.last_registered_job_id.is_none());
    }

    #[test]
    fn test_registration_body_name() {
        let job = define_job(JobOptions {
            name: Some("Test".to_string()),
            ..Default::default()
        });
        let body = job.to_registration_body(None);
        assert_eq!(body["name"], "Test");
    }

    #[test]
    fn test_registration_body_slug() {
        let job = define_job(JobOptions {
            slug: Some("test".to_string()),
            ..Default::default()
        });
        let body = job.to_registration_body(None);
        assert_eq!(body["slug"], "test");
    }

    #[test]
    fn test_registration_body_project_id_param() {
        let job = define_job(JobOptions::default());
        let body = job.to_registration_body(Some("proj-1"));
        assert_eq!(body["project_id"], "proj-1");
    }

    #[test]
    fn test_registration_body_project_id_from_opts() {
        let job = define_job(JobOptions {
            project_id: Some("proj-2".to_string()),
            ..Default::default()
        });
        let body = job.to_registration_body(None);
        assert_eq!(body["project_id"], "proj-2");
    }

    #[test]
    fn test_registration_body_param_overrides_opts() {
        let job = define_job(JobOptions {
            project_id: Some("proj-opts".to_string()),
            ..Default::default()
        });
        let body = job.to_registration_body(Some("proj-param"));
        assert_eq!(body["project_id"], "proj-param");
    }

    #[test]
    fn test_registration_body_cron() {
        let job = define_job(JobOptions {
            cron: Some("*/5 * * * *".to_string()),
            ..Default::default()
        });
        let body = job.to_registration_body(None);
        assert_eq!(body["cron"], "*/5 * * * *");
    }

    #[test]
    fn test_registration_body_max_attempts() {
        let job = define_job(JobOptions {
            max_attempts: Some(3),
            ..Default::default()
        });
        let body = job.to_registration_body(None);
        assert_eq!(body["max_attempts"], 3);
    }

    #[test]
    fn test_registration_body_timeout_secs() {
        let job = define_job(JobOptions {
            timeout_secs: Some(60),
            ..Default::default()
        });
        let body = job.to_registration_body(None);
        assert_eq!(body["timeout_secs"], 60);
    }

    #[test]
    fn test_registration_body_omits_none() {
        let job = define_job(JobOptions {
            name: Some("Test".to_string()),
            ..Default::default()
        });
        let body = job.to_registration_body(None);
        assert!(body.get("cron").is_none());
        assert!(body.get("timeout_secs").is_none());
        assert!(body.get("max_attempts").is_none());
    }

    #[test]
    fn test_registration_body_endpoint_url() {
        let job = define_job(JobOptions {
            endpoint_url: Some("https://example.com/handler".to_string()),
            ..Default::default()
        });
        let body = job.to_registration_body(None);
        assert_eq!(body["endpoint_url"], "https://example.com/handler");
    }

    #[test]
    fn test_registration_body_tags() {
        let job = define_job(JobOptions {
            tags: Some(vec!["a".to_string(), "b".to_string()]),
            ..Default::default()
        });
        let body = job.to_registration_body(None);
        assert_eq!(body["tags"], json!(["a", "b"]));
    }

    #[test]
    fn test_registration_body_description() {
        let job = define_job(JobOptions {
            description: Some("My job".to_string()),
            ..Default::default()
        });
        let body = job.to_registration_body(None);
        assert_eq!(body["description"], "My job");
    }

    #[test]
    fn test_trigger_job_input_default() {
        let input = TriggerJobInput::default();
        assert!(input.job_id.is_none());
        assert!(input.payload.is_none());
        assert!(input.idempotency_key.is_none());
        assert!(input.priority.is_none());
        assert!(input.dry_run.is_none());
        assert!(input.metadata.is_none());
        assert!(input.scheduled_at.is_none());
    }

    #[test]
    fn test_registration_body_max_concurrency() {
        let job = define_job(JobOptions {
            max_concurrency: Some(5),
            ..Default::default()
        });
        let body = job.to_registration_body(None);
        assert_eq!(body["max_concurrency"], 5);
    }

    #[test]
    fn test_registration_body_rate_limit() {
        let job = define_job(JobOptions {
            rate_limit_max: Some(100),
            rate_limit_window_secs: Some(60),
            ..Default::default()
        });
        let body = job.to_registration_body(None);
        assert_eq!(body["rate_limit_max"], 100);
        assert_eq!(body["rate_limit_window_secs"], 60);
    }
}
