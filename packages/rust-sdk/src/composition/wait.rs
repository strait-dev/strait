use crate::errors::StraitError;
use crate::fsm::is_terminal_run_status;
use std::future::Future;
use tokio::time::{sleep, Duration, Instant};

#[derive(Debug, Clone)]
pub struct WaitForRunOptions {
    pub timeout_ms: u64,
    pub initial_delay_ms: u64,
    pub max_delay_ms: u64,
    pub factor: f64,
}

impl Default for WaitForRunOptions {
    fn default() -> Self {
        Self {
            timeout_ms: 300_000,
            initial_delay_ms: 500,
            max_delay_ms: 10_000,
            factor: 1.5,
        }
    }
}

pub async fn wait_for_run<T, F, Fut, S>(
    run_id: &str,
    get_run: F,
    get_status: S,
    opts: Option<&WaitForRunOptions>,
) -> Result<T, StraitError>
where
    F: Fn(&str) -> Fut,
    Fut: Future<Output = Result<T, StraitError>>,
    S: Fn(&T) -> &str,
{
    let default_opts = WaitForRunOptions::default();
    let opts = opts.unwrap_or(&default_opts);
    let start = Instant::now();
    let mut delay = opts.initial_delay_ms;

    loop {
        let run = get_run(run_id).await?;
        let status_str = get_status(&run);

        // Check if terminal (simple string match since we don't parse into enum here)
        let terminal_statuses = [
            "completed", "failed", "timed_out", "crashed",
            "system_failed", "canceled", "expired",
        ];
        if terminal_statuses.contains(&status_str) {
            return Ok(run);
        }

        let elapsed = start.elapsed().as_millis() as u64;
        if elapsed >= opts.timeout_ms {
            return Err(StraitError::Timeout {
                message: format!("timed out waiting for run {}", run_id),
                run_id: Some(run_id.to_string()),
                elapsed_ms: Some(elapsed),
            });
        }

        sleep(Duration::from_millis(delay)).await;
        delay = ((delay as f64 * opts.factor) as u64).min(opts.max_delay_ms);
    }
}
