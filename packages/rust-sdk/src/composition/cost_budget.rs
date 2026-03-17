use crate::errors::StraitError;

pub struct CostBudgetOptions {
    pub max_cost_microusd: i64,
    pub on_warning: Option<Box<dyn Fn(i64, i64) + Send + Sync>>,
    pub warning_threshold: f64,
}

impl Default for CostBudgetOptions {
    fn default() -> Self {
        Self {
            max_cost_microusd: 0,
            on_warning: None,
            warning_threshold: 0.8,
        }
    }
}

pub struct CostTracker {
    current_cost: i64,
    options: CostBudgetOptions,
    warning_fired: bool,
}

impl CostTracker {
    pub fn new(options: CostBudgetOptions) -> Self {
        Self {
            current_cost: 0,
            options,
            warning_fired: false,
        }
    }

    pub fn add(&mut self, cost_microusd: i64) -> Result<(), StraitError> {
        self.current_cost += cost_microusd;

        let threshold =
            (self.options.max_cost_microusd as f64 * self.options.warning_threshold) as i64;
        if !self.warning_fired
            && self.options.on_warning.is_some()
            && self.current_cost >= threshold
        {
            self.warning_fired = true;
            if let Some(ref on_warning) = self.options.on_warning {
                on_warning(self.current_cost, self.options.max_cost_microusd);
            }
        }

        if self.current_cost >= self.options.max_cost_microusd {
            return Err(StraitError::CostBudgetExceeded {
                message: format!(
                    "Cost budget exceeded: {} >= {} microusd",
                    self.current_cost, self.options.max_cost_microusd
                ),
                current_cost_microusd: self.current_cost,
                max_cost_microusd: self.options.max_cost_microusd,
            });
        }
        Ok(())
    }

    pub fn current(&self) -> i64 {
        self.current_cost
    }

    pub fn remaining(&self) -> i64 {
        (self.options.max_cost_microusd - self.current_cost).max(0)
    }

    pub fn is_exceeded(&self) -> bool {
        self.current_cost >= self.options.max_cost_microusd
    }
}

pub fn create_cost_tracker(options: CostBudgetOptions) -> CostTracker {
    CostTracker::new(options)
}

pub async fn with_cost_budget<T, F, Fut>(f: F, options: CostBudgetOptions) -> Result<T, StraitError>
where
    F: FnOnce(CostTracker) -> Fut,
    Fut: std::future::Future<Output = Result<T, StraitError>>,
{
    let tracker = create_cost_tracker(options);
    f(tracker).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;
    use std::sync::atomic::{AtomicBool, AtomicI32, Ordering};

    #[test]
    fn test_cost_tracker_basic() {
        let mut tracker = CostTracker::new(CostBudgetOptions {
            max_cost_microusd: 100_000,
            ..Default::default()
        });
        assert!(tracker.add(50_000).is_ok());
        assert_eq!(tracker.current(), 50_000);
        assert_eq!(tracker.remaining(), 50_000);
        assert!(!tracker.is_exceeded());
    }

    #[test]
    fn test_cost_tracker_exceeded() {
        let mut tracker = CostTracker::new(CostBudgetOptions {
            max_cost_microusd: 100_000,
            ..Default::default()
        });
        let result = tracker.add(100_000);
        assert!(result.is_err());
        assert!(tracker.is_exceeded());
    }

    #[test]
    fn test_cost_tracker_warning() {
        let warned = Arc::new(AtomicBool::new(false));
        let w = warned.clone();
        let mut tracker = CostTracker::new(CostBudgetOptions {
            max_cost_microusd: 100_000,
            warning_threshold: 0.8,
            on_warning: Some(Box::new(move |_, _| {
                w.store(true, Ordering::SeqCst);
            })),
        });
        assert!(tracker.add(50_000).is_ok());
        assert!(!warned.load(Ordering::SeqCst));
        assert!(tracker.add(30_000).is_ok());
        assert!(warned.load(Ordering::SeqCst));
    }

    #[test]
    fn test_cost_tracker_remaining_zero() {
        let mut tracker = CostTracker::new(CostBudgetOptions {
            max_cost_microusd: 50_000,
            ..Default::default()
        });
        let _ = tracker.add(60_000);
        assert_eq!(tracker.remaining(), 0);
    }

    #[test]
    fn test_warning_fires_once() {
        let count = Arc::new(AtomicI32::new(0));
        let c = count.clone();
        let mut tracker = CostTracker::new(CostBudgetOptions {
            max_cost_microusd: 100_000,
            warning_threshold: 0.5,
            on_warning: Some(Box::new(move |_, _| {
                c.fetch_add(1, Ordering::SeqCst);
            })),
        });
        assert!(tracker.add(60_000).is_ok());
        assert!(tracker.add(10_000).is_ok());
        assert_eq!(count.load(Ordering::SeqCst), 1);
    }

    #[test]
    fn test_cost_tracker_multiple_adds() {
        let mut tracker = CostTracker::new(CostBudgetOptions {
            max_cost_microusd: 100_000,
            ..Default::default()
        });
        assert!(tracker.add(30_000).is_ok());
        assert!(tracker.add(30_000).is_ok());
        assert_eq!(tracker.current(), 60_000);
        assert_eq!(tracker.remaining(), 40_000);
    }

    #[test]
    fn test_cost_tracker_exact_budget() {
        let mut tracker = CostTracker::new(CostBudgetOptions {
            max_cost_microusd: 50_000,
            ..Default::default()
        });
        // Exact budget should trigger exceeded
        let result = tracker.add(50_000);
        assert!(result.is_err());
        assert!(tracker.is_exceeded());
    }

    #[tokio::test]
    async fn test_with_cost_budget() {
        let result = with_cost_budget(
            |mut tracker| async move {
                tracker.add(10_000)?;
                Ok(tracker.current())
            },
            CostBudgetOptions {
                max_cost_microusd: 100_000,
                ..Default::default()
            },
        )
        .await;
        assert_eq!(result.unwrap(), 10_000);
    }

    #[tokio::test]
    async fn test_with_cost_budget_exceeded() {
        let result: Result<(), StraitError> = with_cost_budget(
            |mut tracker| async move {
                tracker.add(200_000)?;
                Ok(())
            },
            CostBudgetOptions {
                max_cost_microusd: 100_000,
                ..Default::default()
            },
        )
        .await;
        assert!(result.is_err());
    }
}
