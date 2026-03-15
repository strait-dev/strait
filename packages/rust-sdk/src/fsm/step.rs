use crate::errors::StraitError;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum StepRunStatus {
    Pending,
    Waiting,
    Running,
    Completed,
    Failed,
    Skipped,
    Canceled,
}

impl StepRunStatus {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Pending => "pending",
            Self::Waiting => "waiting",
            Self::Running => "running",
            Self::Completed => "completed",
            Self::Failed => "failed",
            Self::Skipped => "skipped",
            Self::Canceled => "canceled",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum StepRunEvent {
    Wait,
    Start,
    Complete,
    Fail,
    Skip,
    Cancel,
}

pub fn can_transition_step_run(from: StepRunStatus, event: StepRunEvent) -> bool {
    transition_step_run(from, event).is_ok()
}

pub fn transition_step_run(
    from: StepRunStatus,
    event: StepRunEvent,
) -> Result<StepRunStatus, StraitError> {
    use StepRunStatus::*;
    use StepRunEvent::*;

    let to = match (from, event) {
        (Pending, Wait) => Waiting,
        (Pending, Start) => Running,
        (Pending, Skip) => Skipped,
        (Pending, Cancel) => Canceled,
        (Waiting, Start) => Running,
        (Waiting, Cancel) => Canceled,
        (Running, Complete) => Completed,
        (Running, Fail) => Failed,
        (Running, Cancel) => Canceled,
        _ => {
            return Err(StraitError::Validation {
                message: format!("invalid step run transition: {:?} + {:?}", from, event),
                issues: vec![],
            });
        }
    };
    Ok(to)
}

pub fn is_terminal_step_run_status(status: StepRunStatus) -> bool {
    matches!(
        status,
        StepRunStatus::Completed
            | StepRunStatus::Failed
            | StepRunStatus::Skipped
            | StepRunStatus::Canceled
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    // Transition tests
    #[test]
    fn test_pending_wait() { assert_eq!(transition_step_run(StepRunStatus::Pending, StepRunEvent::Wait).unwrap(), StepRunStatus::Waiting); }
    #[test]
    fn test_pending_start() { assert_eq!(transition_step_run(StepRunStatus::Pending, StepRunEvent::Start).unwrap(), StepRunStatus::Running); }
    #[test]
    fn test_pending_skip() { assert_eq!(transition_step_run(StepRunStatus::Pending, StepRunEvent::Skip).unwrap(), StepRunStatus::Skipped); }
    #[test]
    fn test_pending_cancel() { assert_eq!(transition_step_run(StepRunStatus::Pending, StepRunEvent::Cancel).unwrap(), StepRunStatus::Canceled); }
    #[test]
    fn test_waiting_start() { assert_eq!(transition_step_run(StepRunStatus::Waiting, StepRunEvent::Start).unwrap(), StepRunStatus::Running); }
    #[test]
    fn test_waiting_cancel() { assert_eq!(transition_step_run(StepRunStatus::Waiting, StepRunEvent::Cancel).unwrap(), StepRunStatus::Canceled); }
    #[test]
    fn test_running_complete() { assert_eq!(transition_step_run(StepRunStatus::Running, StepRunEvent::Complete).unwrap(), StepRunStatus::Completed); }
    #[test]
    fn test_running_fail() { assert_eq!(transition_step_run(StepRunStatus::Running, StepRunEvent::Fail).unwrap(), StepRunStatus::Failed); }
    #[test]
    fn test_running_cancel() { assert_eq!(transition_step_run(StepRunStatus::Running, StepRunEvent::Cancel).unwrap(), StepRunStatus::Canceled); }

    // Invalid transitions
    #[test]
    fn test_invalid_completed_start() { assert!(transition_step_run(StepRunStatus::Completed, StepRunEvent::Start).is_err()); }
    #[test]
    fn test_invalid_failed_complete() { assert!(transition_step_run(StepRunStatus::Failed, StepRunEvent::Complete).is_err()); }
    #[test]
    fn test_invalid_skipped_start() { assert!(transition_step_run(StepRunStatus::Skipped, StepRunEvent::Start).is_err()); }
    #[test]
    fn test_invalid_canceled_start() { assert!(transition_step_run(StepRunStatus::Canceled, StepRunEvent::Start).is_err()); }
    #[test]
    fn test_invalid_waiting_complete() { assert!(transition_step_run(StepRunStatus::Waiting, StepRunEvent::Complete).is_err()); }

    // can_transition tests
    #[test]
    fn test_can_transition_true() { assert!(can_transition_step_run(StepRunStatus::Pending, StepRunEvent::Start)); }
    #[test]
    fn test_can_transition_false() { assert!(!can_transition_step_run(StepRunStatus::Completed, StepRunEvent::Start)); }
    #[test]
    fn test_can_transition_running_fail() { assert!(can_transition_step_run(StepRunStatus::Running, StepRunEvent::Fail)); }

    // Terminal status tests
    #[test]
    fn test_terminal_completed() { assert!(is_terminal_step_run_status(StepRunStatus::Completed)); }
    #[test]
    fn test_terminal_failed() { assert!(is_terminal_step_run_status(StepRunStatus::Failed)); }
    #[test]
    fn test_terminal_skipped() { assert!(is_terminal_step_run_status(StepRunStatus::Skipped)); }
    #[test]
    fn test_terminal_canceled() { assert!(is_terminal_step_run_status(StepRunStatus::Canceled)); }
    #[test]
    fn test_not_terminal_pending() { assert!(!is_terminal_step_run_status(StepRunStatus::Pending)); }
    #[test]
    fn test_not_terminal_waiting() { assert!(!is_terminal_step_run_status(StepRunStatus::Waiting)); }
    #[test]
    fn test_not_terminal_running() { assert!(!is_terminal_step_run_status(StepRunStatus::Running)); }

    // as_str tests
    #[test]
    fn test_status_as_str_pending() { assert_eq!(StepRunStatus::Pending.as_str(), "pending"); }
    #[test]
    fn test_status_as_str_waiting() { assert_eq!(StepRunStatus::Waiting.as_str(), "waiting"); }
    #[test]
    fn test_status_as_str_running() { assert_eq!(StepRunStatus::Running.as_str(), "running"); }
    #[test]
    fn test_status_as_str_completed() { assert_eq!(StepRunStatus::Completed.as_str(), "completed"); }
    #[test]
    fn test_status_as_str_failed() { assert_eq!(StepRunStatus::Failed.as_str(), "failed"); }
    #[test]
    fn test_status_as_str_skipped() { assert_eq!(StepRunStatus::Skipped.as_str(), "skipped"); }
    #[test]
    fn test_status_as_str_canceled() { assert_eq!(StepRunStatus::Canceled.as_str(), "canceled"); }
}
