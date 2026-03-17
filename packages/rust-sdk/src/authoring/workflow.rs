use super::dag_validation::validate_dag;
use super::steps::Step;
use serde_json::{Value, json};

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
    pub fn to_registration_body(
        &self,
        project_id: Option<&str>,
    ) -> Result<Value, crate::errors::StraitError> {
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
        if let Some(v) = &pid {
            body["project_id"] = json!(v);
        }
        if let Some(v) = &self.opts.description {
            body["description"] = json!(v);
        }
        if let Some(v) = &self.opts.tags {
            body["tags"] = json!(v);
        }
        if let Some(v) = &self.opts.environment_id {
            body["environment_id"] = json!(v);
        }
        if let Some(v) = self.opts.max_concurrent_runs {
            body["max_concurrent_runs"] = json!(v);
        }
        if let Some(v) = self.opts.max_parallel_steps {
            body["max_parallel_steps"] = json!(v);
        }
        if let Some(v) = self.opts.timeout_secs {
            body["timeout_secs"] = json!(v);
        }
        if let Some(v) = self.opts.max_attempts {
            body["max_attempts"] = json!(v);
        }
        if let Some(v) = &self.opts.retry_strategy {
            body["retry_strategy"] = json!(v);
        }
        if let Some(v) = &self.opts.cron {
            body["cron"] = json!(v);
        }
        if let Some(v) = &self.opts.timezone {
            body["timezone"] = json!(v);
        }
        if let Some(v) = &self.opts.webhook_url {
            body["webhook_url"] = json!(v);
        }
        if let Some(v) = &self.opts.webhook_secret {
            body["webhook_secret"] = json!(v);
        }

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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::authoring::steps::{BaseStepOptions, job_step};

    #[test]
    fn test_define_workflow_kind() {
        let wf = define_workflow(WorkflowOptions::default());
        assert_eq!(wf.kind, "workflow");
    }

    #[test]
    fn test_define_workflow_slug() {
        let wf = define_workflow(WorkflowOptions {
            slug: Some("wf".to_string()),
            ..Default::default()
        });
        assert_eq!(wf.slug, Some("wf".to_string()));
    }

    #[test]
    fn test_define_workflow_slug_none() {
        let wf = define_workflow(WorkflowOptions::default());
        assert!(wf.slug.is_none());
    }

    #[test]
    fn test_define_workflow_last_registered_none() {
        let wf = define_workflow(WorkflowOptions::default());
        assert!(wf.last_registered_workflow_id.is_none());
    }

    #[test]
    fn test_registration_body_name() {
        let wf = define_workflow(WorkflowOptions {
            name: Some("WF".to_string()),
            ..Default::default()
        });
        let body = wf.to_registration_body(None).unwrap();
        assert_eq!(body["name"], "WF");
    }

    #[test]
    fn test_registration_body_project_id() {
        let wf = define_workflow(WorkflowOptions::default());
        let body = wf.to_registration_body(Some("proj-1")).unwrap();
        assert_eq!(body["project_id"], "proj-1");
    }

    #[test]
    fn test_registration_body_project_from_opts() {
        let wf = define_workflow(WorkflowOptions {
            project_id: Some("proj-2".to_string()),
            ..Default::default()
        });
        let body = wf.to_registration_body(None).unwrap();
        assert_eq!(body["project_id"], "proj-2");
    }

    #[test]
    fn test_registration_body_with_steps() {
        let steps = vec![
            job_step("s1", "j1", BaseStepOptions::default()),
            job_step(
                "s2",
                "j2",
                BaseStepOptions {
                    depends_on: vec!["s1".to_string()],
                    ..Default::default()
                },
            ),
        ];
        let wf = define_workflow(WorkflowOptions {
            name: Some("WF".to_string()),
            steps: Some(steps),
            ..Default::default()
        });
        let body = wf.to_registration_body(None).unwrap();
        assert!(body.get("steps").is_some());
        assert_eq!(body["steps"].as_array().unwrap().len(), 2);
    }

    #[test]
    fn test_registration_body_invalid_dag() {
        let steps = vec![
            job_step(
                "a",
                "j1",
                BaseStepOptions {
                    depends_on: vec!["b".to_string()],
                    ..Default::default()
                },
            ),
            job_step(
                "b",
                "j2",
                BaseStepOptions {
                    depends_on: vec!["a".to_string()],
                    ..Default::default()
                },
            ),
        ];
        let wf = define_workflow(WorkflowOptions {
            steps: Some(steps),
            ..Default::default()
        });
        assert!(wf.to_registration_body(None).is_err());
    }

    #[test]
    fn test_registration_body_no_steps() {
        let wf = define_workflow(WorkflowOptions::default());
        let body = wf.to_registration_body(None).unwrap();
        assert!(body.get("steps").is_none());
    }

    #[test]
    fn test_registration_body_timeout() {
        let wf = define_workflow(WorkflowOptions {
            timeout_secs: Some(300),
            ..Default::default()
        });
        let body = wf.to_registration_body(None).unwrap();
        assert_eq!(body["timeout_secs"], 300);
    }

    #[test]
    fn test_registration_body_cron() {
        let wf = define_workflow(WorkflowOptions {
            cron: Some("0 * * * *".to_string()),
            ..Default::default()
        });
        let body = wf.to_registration_body(None).unwrap();
        assert_eq!(body["cron"], "0 * * * *");
    }

    #[test]
    fn test_registration_body_omits_none() {
        let wf = define_workflow(WorkflowOptions {
            name: Some("WF".to_string()),
            ..Default::default()
        });
        let body = wf.to_registration_body(None).unwrap();
        assert!(body.get("cron").is_none());
        assert!(body.get("timeout_secs").is_none());
        assert!(body.get("max_attempts").is_none());
    }

    #[test]
    fn test_trigger_workflow_input_default() {
        let input = TriggerWorkflowInput::default();
        assert!(input.workflow_id.is_none());
        assert!(input.payload.is_none());
        assert!(input.idempotency_key.is_none());
        assert!(input.step_overrides.is_none());
    }
}
