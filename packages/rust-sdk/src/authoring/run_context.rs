use serde_json::Value;
use std::future::Future;
use std::pin::Pin;
use std::sync::Arc;

/// Type alias for async callback results.
pub type BoxFuture<T> = Pin<Box<dyn Future<Output = T> + Send>>;
pub type AsyncResult = Result<Value, crate::errors::StraitError>;
pub type AsyncCallback = Arc<dyn Fn(Value) -> BoxFuture<AsyncResult> + Send + Sync>;
pub type AsyncCallbackNoArg = Arc<dyn Fn() -> BoxFuture<AsyncResult> + Send + Sync>;

/// Type alias for state set callback (key, value) -> Result<()>.
pub type AsyncStateSetCallback =
    Arc<dyn Fn(String, Value) -> BoxFuture<Result<(), crate::errors::StraitError>> + Send + Sync>;

/// KV state store operations for a run.
#[derive(Clone)]
pub struct RunContextState {
    pub get: Arc<dyn Fn(String) -> BoxFuture<AsyncResult> + Send + Sync>,
    pub set: AsyncStateSetCallback,
    pub delete:
        Arc<dyn Fn(String) -> BoxFuture<Result<(), crate::errors::StraitError>> + Send + Sync>,
    pub list: Arc<dyn Fn() -> BoxFuture<AsyncResult> + Send + Sync>,
}

/// RunContext is the context object passed to a job's run handler.
#[derive(Clone)]
pub struct RunContext {
    pub run_id: String,
    pub attempt: u32,
    pub checkpoint: Option<AsyncCallback>,
    pub report_progress: Option<AsyncCallback>,
    pub heartbeat: Option<AsyncCallbackNoArg>,
    pub report_usage: Option<AsyncCallback>,
    pub log_tool_call: Option<AsyncCallback>,
    pub save_output: Option<AsyncCallback>,
    pub state: Option<RunContextState>,
    pub stream_chunk: Option<AsyncCallback>,
    pub wait_for_event: Option<AsyncCallback>,
    pub spawn: Option<AsyncCallback>,
    pub continue_run: Option<AsyncCallback>,
    pub annotate: Option<AsyncCallback>,
    pub complete: Option<AsyncCallback>,
    pub fail: Option<AsyncCallback>,
}

impl std::fmt::Debug for RunContext {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("RunContext")
            .field("run_id", &self.run_id)
            .field("attempt", &self.attempt)
            .finish()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_run_context_creation() {
        let ctx = RunContext {
            run_id: "run-123".to_string(),
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
        };
        assert_eq!(ctx.run_id, "run-123");
        assert_eq!(ctx.attempt, 1);
    }

    #[test]
    fn test_run_context_debug() {
        let ctx = RunContext {
            run_id: "r1".to_string(),
            attempt: 0,
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
        };
        let debug = format!("{:?}", ctx);
        assert!(debug.contains("r1"));
        assert!(debug.contains("0"));
    }
}
