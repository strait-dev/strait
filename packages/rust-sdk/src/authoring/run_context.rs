#[derive(Debug, Clone)]
pub struct RunContext {
    pub run_id: String,
    pub attempt: u32,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_run_context_creation() {
        let ctx = RunContext {
            run_id: "run-123".to_string(),
            attempt: 1,
        };
        assert_eq!(ctx.run_id, "run-123");
        assert_eq!(ctx.attempt, 1);
    }

    #[test]
    fn test_run_context_clone() {
        let ctx = RunContext {
            run_id: "run-abc".to_string(),
            attempt: 3,
        };
        let cloned = ctx.clone();
        assert_eq!(cloned.run_id, "run-abc");
        assert_eq!(cloned.attempt, 3);
    }

    #[test]
    fn test_run_context_debug() {
        let ctx = RunContext {
            run_id: "r1".to_string(),
            attempt: 0,
        };
        let debug = format!("{:?}", ctx);
        assert!(debug.contains("r1"));
        assert!(debug.contains("0"));
    }
}
