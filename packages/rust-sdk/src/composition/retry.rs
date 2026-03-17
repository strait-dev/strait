use crate::errors::StraitError;
use std::future::Future;
use tokio::time::{Duration, sleep};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum JitterStrategy {
    Full,
    None,
}

#[derive(Debug, Clone)]
pub struct RetryOptions {
    pub attempts: u32,
    pub delay_ms: u64,
    pub factor: f64,
    pub max_delay_ms: u64,
    pub jitter: JitterStrategy,
}

impl Default for RetryOptions {
    fn default() -> Self {
        Self {
            attempts: 3,
            delay_ms: 250,
            factor: 2.0,
            max_delay_ms: 30_000,
            jitter: JitterStrategy::Full,
        }
    }
}

fn compute_delay(base_delay: u64, jitter: JitterStrategy) -> u64 {
    match jitter {
        JitterStrategy::Full => {
            use std::time::SystemTime;
            let seed = SystemTime::now()
                .duration_since(SystemTime::UNIX_EPOCH)
                .unwrap_or_default()
                .subsec_nanos() as u64;
            seed % (base_delay + 1)
        }
        JitterStrategy::None => base_delay,
    }
}

pub async fn with_retry<T, F, Fut>(f: F, opts: Option<&RetryOptions>) -> Result<T, StraitError>
where
    F: Fn() -> Fut,
    Fut: Future<Output = Result<T, StraitError>>,
{
    let default_opts = RetryOptions::default();
    let opts = opts.unwrap_or(&default_opts);
    let mut last_error = None;

    for attempt in 0..opts.attempts {
        match f().await {
            Ok(v) => return Ok(v),
            Err(e) => {
                if attempt + 1 >= opts.attempts {
                    return Err(e);
                }
                last_error = Some(e);

                let base = (opts.delay_ms as f64 * opts.factor.powi(attempt as i32)) as u64;
                let capped = base.min(opts.max_delay_ms);
                let delay = compute_delay(capped, opts.jitter);
                sleep(Duration::from_millis(delay)).await;
            }
        }
    }

    Err(last_error.unwrap())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;
    use std::sync::atomic::{AtomicU32, Ordering};

    #[test]
    fn test_retry_options_defaults() {
        let opts = RetryOptions::default();
        assert_eq!(opts.attempts, 3);
        assert_eq!(opts.delay_ms, 250);
        assert_eq!(opts.factor, 2.0);
        assert_eq!(opts.max_delay_ms, 30_000);
        assert_eq!(opts.jitter, JitterStrategy::Full);
    }

    #[test]
    fn test_jitter_strategy_none() {
        assert_eq!(JitterStrategy::None, JitterStrategy::None);
    }

    #[test]
    fn test_jitter_strategy_full() {
        assert_eq!(JitterStrategy::Full, JitterStrategy::Full);
    }

    #[test]
    fn test_jitter_strategies_not_equal() {
        assert_ne!(JitterStrategy::Full, JitterStrategy::None);
    }

    #[test]
    fn test_compute_delay_no_jitter() {
        assert_eq!(compute_delay(100, JitterStrategy::None), 100);
    }

    #[test]
    fn test_compute_delay_no_jitter_zero() {
        assert_eq!(compute_delay(0, JitterStrategy::None), 0);
    }

    #[test]
    fn test_compute_delay_full_jitter_bounded() {
        for _ in 0..100 {
            let delay = compute_delay(1000, JitterStrategy::Full);
            assert!(delay <= 1000);
        }
    }

    #[test]
    fn test_compute_delay_full_jitter_zero_base() {
        let delay = compute_delay(0, JitterStrategy::Full);
        assert_eq!(delay, 0);
    }

    #[tokio::test]
    async fn test_retry_succeeds_first_try() {
        let result = with_retry(|| async { Ok::<_, StraitError>(42) }, None).await;
        assert_eq!(result.unwrap(), 42);
    }

    #[tokio::test]
    async fn test_retry_succeeds_after_failures() {
        let counter = Arc::new(AtomicU32::new(0));
        let c = counter.clone();
        let opts = RetryOptions {
            attempts: 3,
            delay_ms: 1,
            factor: 1.0,
            max_delay_ms: 10,
            jitter: JitterStrategy::None,
        };
        let result = with_retry(
            move || {
                let c = c.clone();
                async move {
                    let n = c.fetch_add(1, Ordering::SeqCst);
                    if n < 2 {
                        Err(StraitError::Transport {
                            message: "fail".to_string(),
                            cause: None,
                        })
                    } else {
                        Ok(42)
                    }
                }
            },
            Some(&opts),
        )
        .await;
        assert_eq!(result.unwrap(), 42);
        assert_eq!(counter.load(Ordering::SeqCst), 3);
    }

    #[tokio::test]
    async fn test_retry_exhausts_attempts() {
        let opts = RetryOptions {
            attempts: 2,
            delay_ms: 1,
            factor: 1.0,
            max_delay_ms: 10,
            jitter: JitterStrategy::None,
        };
        let result = with_retry(
            || async {
                Err::<i32, _>(StraitError::Transport {
                    message: "fail".to_string(),
                    cause: None,
                })
            },
            Some(&opts),
        )
        .await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_retry_single_attempt() {
        let counter = Arc::new(AtomicU32::new(0));
        let c = counter.clone();
        let opts = RetryOptions {
            attempts: 1,
            delay_ms: 1,
            factor: 1.0,
            max_delay_ms: 10,
            jitter: JitterStrategy::None,
        };
        let result = with_retry(
            move || {
                let c = c.clone();
                async move {
                    c.fetch_add(1, Ordering::SeqCst);
                    Err::<i32, _>(StraitError::Transport {
                        message: "fail".to_string(),
                        cause: None,
                    })
                }
            },
            Some(&opts),
        )
        .await;
        assert!(result.is_err());
        assert_eq!(counter.load(Ordering::SeqCst), 1);
    }

    #[tokio::test]
    async fn test_retry_uses_default_opts() {
        let result = with_retry(|| async { Ok::<_, StraitError>("ok") }, None).await;
        assert_eq!(result.unwrap(), "ok");
    }
}
