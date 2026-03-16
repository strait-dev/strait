use crate::authoring::run_context::*;
use crate::errors::StraitError;
use serde_json::{json, Value};
use std::future::Future;
use std::pin::Pin;
use std::sync::Arc;

/// Trait for HTTP client operations needed by RunContext.
pub trait RunContextClient: Send + Sync + 'static {
    fn checkpoint_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn heartbeat_run(
        &self,
        run_id: &str,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn progress_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn log_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn usage_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn tool_call_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn output_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn wait_for_event_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn spawn_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn continue_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn annotate_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn complete_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn fail_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn set_state(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn list_state(
        &self,
        run_id: &str,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn get_state(
        &self,
        run_id: &str,
        key: &str,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn delete_state(
        &self,
        run_id: &str,
        key: &str,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
    fn stream_run(
        &self,
        run_id: &str,
        body: Value,
    ) -> Pin<Box<dyn Future<Output = Result<Value, StraitError>> + Send>>;
}

/// Creates a RunContext wired to HTTP endpoints via the client.
pub fn create_run_context(
    client: Arc<dyn RunContextClient>,
    run_id: String,
    attempt: u32,
) -> RunContext {
    let rid = run_id.clone();
    let c = client.clone();
    let checkpoint: AsyncCallback = Arc::new(move |body: Value| {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move {
            let wrapped = json!({"state": body, "source": "sdk"});
            c.checkpoint_run(&rid, wrapped).await
        })
    });

    let rid = run_id.clone();
    let c = client.clone();
    let heartbeat: AsyncCallbackNoArg = Arc::new(move || {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move { c.heartbeat_run(&rid).await })
    });

    let rid = run_id.clone();
    let c = client.clone();
    let report_progress: AsyncCallback = Arc::new(move |body: Value| {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move { c.progress_run(&rid, body).await })
    });

    let rid = run_id.clone();
    let c = client.clone();
    let report_usage: AsyncCallback = Arc::new(move |body: Value| {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move { c.usage_run(&rid, body).await })
    });

    let rid = run_id.clone();
    let c = client.clone();
    let log_tool_call: AsyncCallback = Arc::new(move |body: Value| {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move { c.tool_call_run(&rid, body).await })
    });

    let rid = run_id.clone();
    let c = client.clone();
    let save_output: AsyncCallback = Arc::new(move |body: Value| {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move { c.output_run(&rid, body).await })
    });

    let rid = run_id.clone();
    let c = client.clone();
    let wait_for_event: AsyncCallback = Arc::new(move |body: Value| {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move { c.wait_for_event_run(&rid, body).await })
    });

    let rid = run_id.clone();
    let c = client.clone();
    let spawn: AsyncCallback = Arc::new(move |body: Value| {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move { c.spawn_run(&rid, body).await })
    });

    let rid = run_id.clone();
    let c = client.clone();
    let continue_fn: AsyncCallback = Arc::new(move |body: Value| {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move { c.continue_run(&rid, body).await })
    });

    let rid = run_id.clone();
    let c = client.clone();
    let annotate: AsyncCallback = Arc::new(move |body: Value| {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move { c.annotate_run(&rid, body).await })
    });

    let rid = run_id.clone();
    let c = client.clone();
    let complete: AsyncCallback = Arc::new(move |body: Value| {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move { c.complete_run(&rid, body).await })
    });

    let rid = run_id.clone();
    let c = client.clone();
    let fail: AsyncCallback = Arc::new(move |body: Value| {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move { c.fail_run(&rid, body).await })
    });

    let rid = run_id.clone();
    let c = client.clone();
    let stream_chunk: AsyncCallback = Arc::new(move |body: Value| {
        let c = c.clone();
        let rid = rid.clone();
        Box::pin(async move { c.stream_run(&rid, body).await })
    });

    // State KV store
    let rid_get = run_id.clone();
    let c_get = client.clone();
    let state_get = Arc::new(move |key: String| -> BoxFuture<AsyncResult> {
        let c = c_get.clone();
        let rid = rid_get.clone();
        Box::pin(async move { c.get_state(&rid, &key).await })
    });

    let rid_set = run_id.clone();
    let c_set = client.clone();
    let state_set = Arc::new(
        move |key: String, value: Value| -> BoxFuture<Result<(), StraitError>> {
            let c = c_set.clone();
            let rid = rid_set.clone();
            Box::pin(async move {
                let body = json!({"key": key, "value": value});
                c.set_state(&rid, body).await.map(|_| ())
            })
        },
    );

    let rid_del = run_id.clone();
    let c_del = client.clone();
    let state_delete = Arc::new(move |key: String| -> BoxFuture<Result<(), StraitError>> {
        let c = c_del.clone();
        let rid = rid_del.clone();
        Box::pin(async move { c.delete_state(&rid, &key).await.map(|_| ()) })
    });

    let rid_list = run_id.clone();
    let c_list = client.clone();
    let state_list = Arc::new(move || -> BoxFuture<AsyncResult> {
        let c = c_list.clone();
        let rid = rid_list.clone();
        Box::pin(async move { c.list_state(&rid).await })
    });

    let state = RunContextState {
        get: state_get,
        set: state_set,
        delete: state_delete,
        list: state_list,
    };

    RunContext {
        run_id,
        attempt,
        checkpoint: Some(checkpoint),
        report_progress: Some(report_progress),
        heartbeat: Some(heartbeat),
        report_usage: Some(report_usage),
        log_tool_call: Some(log_tool_call),
        save_output: Some(save_output),
        state: Some(state),
        stream_chunk: Some(stream_chunk),
        wait_for_event: Some(wait_for_event),
        spawn: Some(spawn),
        continue_run: Some(continue_fn),
        annotate: Some(annotate),
        complete: Some(complete),
        fail: Some(fail),
    }
}
