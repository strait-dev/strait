use crate::errors::StraitError;
use std::future::Future;
use tokio::time::{sleep, Duration};

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

pub async fn with_retry<T, F, Fut>(
    f: F,
    opts: Option<&RetryOptions>,
) -> Result<T, StraitError>
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
