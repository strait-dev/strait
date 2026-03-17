use crate::errors::StraitError;
use std::future::Future;
use tokio::time::{Duration, Instant, sleep};

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
            "completed",
            "failed",
            "timed_out",
            "crashed",
            "system_failed",
            "canceled",
            "expired",
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_wait_for_run_options_default() {
        let opts = WaitForRunOptions::default();
        assert_eq!(opts.timeout_ms, 300_000);
        assert_eq!(opts.initial_delay_ms, 500);
        assert_eq!(opts.max_delay_ms, 10_000);
        assert_eq!(opts.factor, 1.5);
    }

    #[tokio::test]
    async fn test_wait_for_run_already_completed() {
        let result = wait_for_run(
            "run-1",
            |_id| async { Ok::<_, StraitError>("completed") },
            |s| *s,
            None,
        )
        .await;
        assert_eq!(result.unwrap(), "completed");
    }

    #[tokio::test]
    async fn test_wait_for_run_already_failed() {
        let result = wait_for_run(
            "run-1",
            |_id| async { Ok::<_, StraitError>("failed") },
            |s| *s,
            None,
        )
        .await;
        assert_eq!(result.unwrap(), "failed");
    }

    #[tokio::test]
    async fn test_wait_for_run_timeout() {
        let opts = WaitForRunOptions {
            timeout_ms: 50,
            initial_delay_ms: 10,
            max_delay_ms: 20,
            factor: 1.0,
        };
        let result = wait_for_run(
            "run-1",
            |_id| async { Ok::<_, StraitError>("running") },
            |s| *s,
            Some(&opts),
        )
        .await;
        assert!(result.is_err());
        match result.unwrap_err() {
            StraitError::Timeout { run_id, .. } => {
                assert_eq!(run_id, Some("run-1".to_string()));
            }
            _ => panic!("expected Timeout"),
        }
    }

    #[tokio::test]
    async fn test_wait_for_run_get_error() {
        let result = wait_for_run(
            "run-1",
            |_id| async {
                Err::<String, _>(StraitError::NotFound {
                    status: 404,
                    message: "not found".to_string(),
                    body: None,
                })
            },
            |s: &String| s.as_str(),
            None,
        )
        .await;
        assert!(result.is_err());
        assert!(matches!(result.unwrap_err(), StraitError::NotFound { .. }));
    }

    #[tokio::test]
    async fn test_wait_for_run_canceled() {
        let result = wait_for_run(
            "run-1",
            |_id| async { Ok::<_, StraitError>("canceled") },
            |s| *s,
            None,
        )
        .await;
        assert_eq!(result.unwrap(), "canceled");
    }

    #[tokio::test]
    async fn test_wait_for_run_timed_out_status() {
        let result = wait_for_run(
            "run-1",
            |_id| async { Ok::<_, StraitError>("timed_out") },
            |s| *s,
            None,
        )
        .await;
        assert_eq!(result.unwrap(), "timed_out");
    }
}
