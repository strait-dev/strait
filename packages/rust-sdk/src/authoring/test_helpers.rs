use crate::authoring::run_context::*;
use crate::authoring::run_context_client::{create_run_context, RunContextClient};
use crate::errors::StraitError;
use serde_json::{json, Value};
use std::collections::HashMap;
use std::future::Future;
use std::pin::Pin;
use std::sync::{Arc, Mutex};

/// Captures all operations on a test RunContext.
#[derive(Debug, Default)]
pub struct TestRunRecord {
    pub checkpoints: Vec<Value>,
    pub logs: Vec<Value>,
    pub usage_reports: Vec<Value>,
    pub tool_calls: Vec<Value>,
    pub outputs: Vec<Value>,
    pub progress_updates: Vec<Value>,
    pub state_store: HashMap<String, Value>,
    pub stream_chunks: Vec<Value>,
    pub heartbeats: u32,
    pub spawns: Vec<Value>,
    pub events: Vec<Value>,
    pub annotations: Vec<Value>,
    pub continuations: Vec<Value>,
    pub completed: bool,
    pub failed: bool,
    pub fail_error: Option<String>,
    pub result: Option<Value>,
}

struct MockClient {
    record: Arc<Mutex<TestRunRecord>>,
}

impl RunContextClient for MockClient {
    fn checkpoint_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            let mut r = record.lock().unwrap();
            if let Some(state) = body.get("state") {
                r.checkpoints.push(state.clone());
            }
            Ok(json!(null))
        })
    }

    fn heartbeat_run(
        &self,
        _: &str,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            record.lock().unwrap().heartbeats += 1;
            Ok(json!(null))
        })
    }

    fn progress_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            record.lock().unwrap().progress_updates.push(body);
            Ok(json!(null))
        })
    }

    fn log_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            record.lock().unwrap().logs.push(body);
            Ok(json!(null))
        })
    }

    fn usage_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            record.lock().unwrap().usage_reports.push(body);
            Ok(json!(null))
        })
    }

    fn tool_call_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            record.lock().unwrap().tool_calls.push(body);
            Ok(json!(null))
        })
    }

    fn output_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            record.lock().unwrap().outputs.push(body);
            Ok(json!(null))
        })
    }

    fn wait_for_event_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            record.lock().unwrap().events.push(body.clone());
            Ok(json!({"status": "waiting", "event_key": body["event_key"]}))
        })
    }

    fn spawn_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            let mut r = record.lock().unwrap();
            r.spawns.push(body);
            Ok(json!({"id": format!("spawn_{}", r.spawns.len())}))
        })
    }

    fn continue_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            let mut r = record.lock().unwrap();
            r.continuations.push(body);
            Ok(json!({"id": format!("continue_{}", r.continuations.len())}))
        })
    }

    fn annotate_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            record.lock().unwrap().annotations.push(body);
            Ok(json!(null))
        })
    }

    fn complete_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            let mut r = record.lock().unwrap();
            r.completed = true;
            if let Some(result) = body.get("result") {
                r.result = Some(result.clone());
            }
            Ok(json!(null))
        })
    }

    fn fail_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            let mut r = record.lock().unwrap();
            r.failed = true;
            r.fail_error = body
                .get("error")
                .and_then(|e| e.as_str())
                .map(|s| s.to_string());
            Ok(json!(null))
        })
    }

    fn set_state(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            let mut r = record.lock().unwrap();
            if let (Some(key), Some(value)) = (
                body.get("key").and_then(|k| k.as_str()),
                body.get("value"),
            ) {
                r.state_store.insert(key.to_string(), value.clone());
            }
            Ok(json!(null))
        })
    }

    fn list_state(
        &self,
        _: &str,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            let r = record.lock().unwrap();
            let entries: Vec<Value> = r
                .state_store
                .iter()
                .map(|(k, v)| json!({"key": k, "value": v}))
                .collect();
            Ok(json!(entries))
        })
    }

    fn get_state(
        &self,
        _: &str,
        key: &str,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        let key = key.to_string();
        Box::pin(async move {
            let r = record.lock().unwrap();
            Ok(r.state_store.get(&key).cloned().unwrap_or(json!(null)))
        })
    }

    fn delete_state(
        &self,
        _: &str,
        key: &str,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        let key = key.to_string();
        Box::pin(async move {
            record.lock().unwrap().state_store.remove(&key);
            Ok(json!(null))
        })
    }

    fn stream_run(
        &self,
        _: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>> {
        let record = self.record.clone();
        Box::pin(async move {
            record.lock().unwrap().stream_chunks.push(body);
            Ok(json!(null))
        })
    }
}

/// Creates an in-memory RunContext and TestRunRecord for testing.
pub fn create_test_context(
    run_id: impl Into<String>,
    attempt: u32,
) -> (RunContext, Arc<Mutex<TestRunRecord>>) {
    let record = Arc::new(Mutex::new(TestRunRecord::default()));
    let mock = Arc::new(MockClient {
        record: record.clone(),
    });
    let ctx = create_run_context(mock, run_id.into(), attempt);
    (ctx, record)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_create_test_context() {
        let (ctx, record) = create_test_context("test-run", 1);
        assert_eq!(ctx.run_id, "test-run");
        assert_eq!(ctx.attempt, 1);
        assert_eq!(record.lock().unwrap().heartbeats, 0);
    }

    #[tokio::test]
    async fn test_checkpoint() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref checkpoint) = ctx.checkpoint {
            checkpoint(json!({"step": 1})).await.unwrap();
        }
        assert_eq!(record.lock().unwrap().checkpoints.len(), 1);
    }

    #[tokio::test]
    async fn test_heartbeat() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref heartbeat) = ctx.heartbeat {
            heartbeat().await.unwrap();
            heartbeat().await.unwrap();
        }
        assert_eq!(record.lock().unwrap().heartbeats, 2);
    }

    #[tokio::test]
    async fn test_state_operations() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref state) = ctx.state {
            (state.set)("key1".to_string(), json!("val1"))
                .await
                .unwrap();
            let val = (state.get)("key1".to_string()).await.unwrap();
            assert_eq!(val, json!("val1"));
            (state.delete)("key1".to_string()).await.unwrap();
        }
        assert!(record.lock().unwrap().state_store.is_empty());
    }

    #[tokio::test]
    async fn test_stream_chunks() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref stream) = ctx.stream_chunk {
            stream(json!({"chunk": "hello", "stream_id": "s1"}))
                .await
                .unwrap();
        }
        assert_eq!(record.lock().unwrap().stream_chunks.len(), 1);
    }

    #[tokio::test]
    async fn test_complete() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref complete) = ctx.complete {
            complete(json!({"result": {"answer": 42}})).await.unwrap();
        }
        let r = record.lock().unwrap();
        assert!(r.completed);
        assert_eq!(r.result, Some(json!({"answer": 42})));
    }

    #[tokio::test]
    async fn test_fail() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref fail) = ctx.fail {
            fail(json!({"error": "boom"})).await.unwrap();
        }
        let r = record.lock().unwrap();
        assert!(r.failed);
        assert_eq!(r.fail_error, Some("boom".to_string()));
    }

    #[tokio::test]
    async fn test_spawn() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref spawn) = ctx.spawn {
            let result = spawn(json!({"job_slug": "worker", "project_id": "p1"}))
                .await
                .unwrap();
            assert!(result.get("id").is_some());
        }
        assert_eq!(record.lock().unwrap().spawns.len(), 1);
    }

    #[tokio::test]
    async fn test_report_progress() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref progress) = ctx.report_progress {
            progress(json!({"percent": 50})).await.unwrap();
        }
        assert_eq!(record.lock().unwrap().progress_updates.len(), 1);
    }

    #[tokio::test]
    async fn test_report_usage() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref usage) = ctx.report_usage {
            usage(json!({"tokens": 100})).await.unwrap();
        }
        assert_eq!(record.lock().unwrap().usage_reports.len(), 1);
    }

    #[tokio::test]
    async fn test_log_tool_call() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref log_tc) = ctx.log_tool_call {
            log_tc(json!({"tool": "search", "args": {}})).await.unwrap();
        }
        assert_eq!(record.lock().unwrap().tool_calls.len(), 1);
    }

    #[tokio::test]
    async fn test_save_output() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref save) = ctx.save_output {
            save(json!({"data": "result"})).await.unwrap();
        }
        assert_eq!(record.lock().unwrap().outputs.len(), 1);
    }

    #[tokio::test]
    async fn test_wait_for_event() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref wait) = ctx.wait_for_event {
            let result = wait(json!({"event_key": "approval"})).await.unwrap();
            assert_eq!(result["status"], "waiting");
        }
        assert_eq!(record.lock().unwrap().events.len(), 1);
    }

    #[tokio::test]
    async fn test_annotate() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref annotate) = ctx.annotate {
            annotate(json!({"label": "important"})).await.unwrap();
        }
        assert_eq!(record.lock().unwrap().annotations.len(), 1);
    }

    #[tokio::test]
    async fn test_continue_run() {
        let (ctx, record) = create_test_context("r1", 1);
        if let Some(ref cont) = ctx.continue_run {
            let result = cont(json!({"next_step": "step2"})).await.unwrap();
            assert!(result.get("id").is_some());
        }
        assert_eq!(record.lock().unwrap().continuations.len(), 1);
    }

    #[tokio::test]
    async fn test_state_list() {
        let (ctx, _record) = create_test_context("r1", 1);
        if let Some(ref state) = ctx.state {
            (state.set)("a".to_string(), json!(1)).await.unwrap();
            (state.set)("b".to_string(), json!(2)).await.unwrap();
            let list = (state.list)().await.unwrap();
            assert_eq!(list.as_array().unwrap().len(), 2);
        }
    }
}
