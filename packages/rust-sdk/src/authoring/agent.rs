use crate::authoring::job::{define_job, JobDefinition, JobOptions};
use crate::authoring::run_context::RunContext;
use std::sync::{Arc, Mutex};

/// Agent-specific run context with iteration and cost tracking.
pub struct AgentRunContext {
    pub ctx: RunContext,
    iteration: Arc<Mutex<i32>>,
    accumulated_cost: Arc<Mutex<i64>>,
    max_cost: i64,
}

impl AgentRunContext {
    pub fn iteration(&self) -> i32 {
        *self.iteration.lock().unwrap()
    }

    pub fn accumulated_cost_microusd(&self) -> i64 {
        *self.accumulated_cost.lock().unwrap()
    }

    pub fn is_budget_exceeded(&self) -> bool {
        self.accumulated_cost_microusd() >= self.max_cost
    }
}

/// Options for defining an agent.
pub struct AgentOptions {
    pub name: String,
    pub slug: String,
    pub endpoint_url: String,
    pub project_id: Option<String>,
    pub description: Option<String>,
    pub tags: Option<Vec<String>>,
    pub max_iterations: Option<i32>,
    pub max_cost_microusd: Option<i64>,
    pub auto_checkpoint: Option<bool>,
    pub timeout_secs: Option<u32>,
    pub max_attempts: Option<u32>,
    pub retry_strategy: Option<String>,
}

/// Creates a job definition with agent conventions.
pub fn define_agent(opts: AgentOptions) -> JobDefinition {
    let mut tags = opts.tags.unwrap_or_default();
    if !tags.contains(&"strait.kind:agent".to_string()) {
        tags.push("strait.kind:agent".to_string());
    }

    define_job(JobOptions {
        name: Some(opts.name),
        slug: Some(opts.slug),
        endpoint_url: Some(opts.endpoint_url),
        project_id: opts.project_id,
        description: opts.description,
        tags: Some(tags),
        timeout_secs: Some(opts.timeout_secs.unwrap_or(600)),
        max_attempts: Some(opts.max_attempts.unwrap_or(5)),
        retry_strategy: Some(
            opts.retry_strategy
                .unwrap_or_else(|| "exponential".to_string()),
        ),
        ..Default::default()
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn test_define_agent_defaults() {
        let agent = define_agent(AgentOptions {
            name: "test-agent".to_string(),
            slug: "test-agent".to_string(),
            endpoint_url: "https://example.com".to_string(),
            project_id: Some("proj-1".to_string()),
            description: None,
            tags: None,
            max_iterations: None,
            max_cost_microusd: None,
            auto_checkpoint: None,
            timeout_secs: None,
            max_attempts: None,
            retry_strategy: None,
        });
        let body = agent.to_registration_body(Some("proj-1"));
        let tags = body["tags"].as_array().unwrap();
        assert!(tags.contains(&json!("strait.kind:agent")));
        assert_eq!(body["timeout_secs"], 600);
        assert_eq!(body["max_attempts"], 5);
        assert_eq!(body["retry_strategy"], "exponential");
    }

    #[test]
    fn test_define_agent_custom_tags() {
        let tags = vec!["env:prod".to_string()];
        let agent = define_agent(AgentOptions {
            name: "a".to_string(),
            slug: "a".to_string(),
            endpoint_url: "https://x.com".to_string(),
            project_id: Some("p".to_string()),
            description: None,
            tags: Some(tags),
            max_iterations: None,
            max_cost_microusd: None,
            auto_checkpoint: None,
            timeout_secs: Some(1200),
            max_attempts: Some(3),
            retry_strategy: Some("fixed".to_string()),
        });
        let body = agent.to_registration_body(Some("p"));
        let tags = body["tags"].as_array().unwrap();
        assert!(tags.contains(&json!("strait.kind:agent")));
        assert!(tags.contains(&json!("env:prod")));
        assert_eq!(body["timeout_secs"], 1200);
        assert_eq!(body["max_attempts"], 3);
    }

    #[test]
    fn test_agent_run_context_initial() {
        let agent_ctx = AgentRunContext {
            ctx: RunContext {
                run_id: "r1".to_string(),
                attempt: 1,
                checkpoint: None,
                report_progress: None,
                heartbeat: None,
                report_usage: None,
                log_tool_call: None,
                save_output: None,
                state: None,
                stream_chunk: None,
                wait_for_event: None,
                spawn: None,
                continue_run: None,
                annotate: None,
                complete: None,
                fail: None,
            },
            iteration: Arc::new(Mutex::new(0)),
            accumulated_cost: Arc::new(Mutex::new(0)),
            max_cost: 1_000_000,
        };
        assert_eq!(agent_ctx.iteration(), 0);
        assert_eq!(agent_ctx.accumulated_cost_microusd(), 0);
        assert!(!agent_ctx.is_budget_exceeded());
    }

    #[test]
    fn test_agent_budget_exceeded() {
        let agent_ctx = AgentRunContext {
            ctx: RunContext {
                run_id: "r1".to_string(),
                attempt: 1,
                checkpoint: None,
                report_progress: None,
                heartbeat: None,
                report_usage: None,
                log_tool_call: None,
                save_output: None,
                state: None,
                stream_chunk: None,
                wait_for_event: None,
                spawn: None,
                continue_run: None,
                annotate: None,
                complete: None,
                fail: None,
            },
            iteration: Arc::new(Mutex::new(3)),
            accumulated_cost: Arc::new(Mutex::new(1_000_000)),
            max_cost: 1_000_000,
        };
        assert!(agent_ctx.is_budget_exceeded());
        assert_eq!(agent_ctx.iteration(), 3);
    }

    #[test]
    fn test_define_agent_slug_set() {
        let agent = define_agent(AgentOptions {
            name: "my-agent".to_string(),
            slug: "my-agent-slug".to_string(),
            endpoint_url: "https://example.com".to_string(),
            project_id: None,
            description: Some("An agent".to_string()),
            tags: None,
            max_iterations: None,
            max_cost_microusd: None,
            auto_checkpoint: None,
            timeout_secs: None,
            max_attempts: None,
            retry_strategy: None,
        });
        assert_eq!(agent.slug, Some("my-agent-slug".to_string()));
        assert_eq!(agent.kind, "job");
    }

    #[test]
    fn test_agent_budget_not_exceeded_below() {
        let agent_ctx = AgentRunContext {
            ctx: RunContext {
                run_id: "r1".to_string(),
                attempt: 1,
                checkpoint: None,
                report_progress: None,
                heartbeat: None,
                report_usage: None,
                log_tool_call: None,
                save_output: None,
                state: None,
                stream_chunk: None,
                wait_for_event: None,
                spawn: None,
                continue_run: None,
                annotate: None,
                complete: None,
                fail: None,
            },
            iteration: Arc::new(Mutex::new(1)),
            accumulated_cost: Arc::new(Mutex::new(500_000)),
            max_cost: 1_000_000,
        };
        assert!(!agent_ctx.is_budget_exceeded());
        assert_eq!(agent_ctx.accumulated_cost_microusd(), 500_000);
    }
}
