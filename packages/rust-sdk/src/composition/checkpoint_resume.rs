use crate::authoring::run_context::RunContext;
use serde_json::Value;
use std::sync::{Arc, Mutex};

pub struct CheckpointResumeOptions {
    pub initial_state: Value,
    pub checkpoint_interval: usize,
}

impl Default for CheckpointResumeOptions {
    fn default() -> Self {
        Self {
            initial_state: Value::Object(Default::default()),
            checkpoint_interval: 1,
        }
    }
}

pub async fn with_checkpoint_resume<T, F, Fut>(
    ctx: &RunContext,
    last_checkpoint: Option<Value>,
    f: F,
    _options: CheckpointResumeOptions,
) -> Result<T, crate::errors::StraitError>
where
    F: FnOnce(Value, Box<dyn FnMut(Value) + Send>) -> Fut,
    Fut: std::future::Future<Output = Result<T, crate::errors::StraitError>>,
{
    let current_state = last_checkpoint.unwrap_or(_options.initial_state);

    let state_ref = Arc::new(Mutex::new(current_state.clone()));

    let state_clone = state_ref.clone();

    let update_state = Box::new(move |new_state: Value| {
        *state_clone.lock().unwrap() = new_state;
    });

    let initial = state_ref.lock().unwrap().clone();
    let result = f(initial, update_state).await?;

    // Final checkpoint
    if let Some(ref checkpoint) = ctx.checkpoint {
        let final_state = state_ref.lock().unwrap().clone();
        let _ = checkpoint(final_state).await;
    }

    Ok(result)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::authoring::run_context::RunContext;
    use serde_json::json;

    fn bare_context() -> RunContext {
        RunContext {
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
        }
    }

    #[tokio::test]
    async fn test_checkpoint_resume_initial_state() {
        let ctx = bare_context();

        let result = with_checkpoint_resume(
            &ctx,
            None,
            |state, _update| async move {
                assert_eq!(state, json!({"count": 0}));
                Ok(42)
            },
            CheckpointResumeOptions {
                initial_state: json!({"count": 0}),
                checkpoint_interval: 1,
            },
        )
        .await;
        assert_eq!(result.unwrap(), 42);
    }

    #[tokio::test]
    async fn test_checkpoint_resume_with_last_checkpoint() {
        let ctx = bare_context();

        let result = with_checkpoint_resume(
            &ctx,
            Some(json!({"count": 5})),
            |state, _update| async move {
                assert_eq!(state, json!({"count": 5}));
                Ok("resumed")
            },
            CheckpointResumeOptions {
                initial_state: json!({"count": 0}),
                checkpoint_interval: 1,
            },
        )
        .await;
        assert_eq!(result.unwrap(), "resumed");
    }

    #[tokio::test]
    async fn test_checkpoint_resume_update_state() {
        let ctx = bare_context();

        let result = with_checkpoint_resume(
            &ctx,
            None,
            |state, mut update| async move {
                assert_eq!(state, json!({"step": 0}));
                update(json!({"step": 1}));
                Ok("done")
            },
            CheckpointResumeOptions {
                initial_state: json!({"step": 0}),
                checkpoint_interval: 1,
            },
        )
        .await;
        assert_eq!(result.unwrap(), "done");
    }

    #[tokio::test]
    async fn test_checkpoint_resume_default_options() {
        let ctx = bare_context();

        let result = with_checkpoint_resume(
            &ctx,
            None,
            |state, _update| async move {
                assert_eq!(state, json!({}));
                Ok(true)
            },
            CheckpointResumeOptions::default(),
        )
        .await;
        assert!(result.unwrap());
    }

    #[tokio::test]
    async fn test_checkpoint_resume_last_checkpoint_overrides_initial() {
        let ctx = bare_context();

        let result = with_checkpoint_resume(
            &ctx,
            Some(json!({"resumed": true})),
            |state, _update| async move {
                // last_checkpoint should override initial_state
                assert_eq!(state, json!({"resumed": true}));
                Ok(99)
            },
            CheckpointResumeOptions {
                initial_state: json!({"resumed": false}),
                checkpoint_interval: 5,
            },
        )
        .await;
        assert_eq!(result.unwrap(), 99);
    }
}
