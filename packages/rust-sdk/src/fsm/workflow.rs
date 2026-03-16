use crate::errors::StraitError;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum WorkflowRunStatus {
    Pending,
    Running,
    Paused,
    Completed,
    Failed,
    TimedOut,
    Canceled,
}

impl WorkflowRunStatus {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Pending => "pending",
            Self::Running => "running",
            Self::Paused => "paused",
            Self::Completed => "completed",
            Self::Failed => "failed",
            Self::TimedOut => "timed_out",
            Self::Canceled => "canceled",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum WorkflowRunEvent {
    Start,
    Pause,
    Resume,
    Complete,
    Fail,
    Timeout,
    Cancel,
}

pub fn can_transition_workflow_run(from: WorkflowRunStatus, event: WorkflowRunEvent) -> bool {
    transition_workflow_run(from, event).is_ok()
}

pub fn transition_workflow_run(
    from: WorkflowRunStatus,
    event: WorkflowRunEvent,
) -> Result<WorkflowRunStatus, StraitError> {
    use WorkflowRunEvent::*;
    use WorkflowRunStatus::*;

    let to = match (from, event) {
        (Pending, Start) => Running,
        (Pending, Cancel) => Canceled,
        (Running, Pause) => Paused,
        (Running, Complete) => Completed,
        (Running, Fail) => Failed,
        (Running, Timeout) => TimedOut,
        (Running, Cancel) => Canceled,
        (Paused, Resume) => Running,
        (Paused, Cancel) => Canceled,
        _ => {
            return Err(StraitError::Validation {
                message: format!("invalid workflow run transition: {:?} + {:?}", from, event),
                issues: vec![],
            });
        }
    };
    Ok(to)
}

pub fn is_terminal_workflow_run_status(status: WorkflowRunStatus) -> bool {
    matches!(
        status,
        WorkflowRunStatus::Completed
            | WorkflowRunStatus::Failed
            | WorkflowRunStatus::TimedOut
            | WorkflowRunStatus::Canceled
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    // Transition tests
    #[test]
    fn test_pending_start() {
        assert_eq!(
            transition_workflow_run(WorkflowRunStatus::Pending, WorkflowRunEvent::Start).unwrap(),
            WorkflowRunStatus::Running
        );
    }
    #[test]
    fn test_pending_cancel() {
        assert_eq!(
            transition_workflow_run(WorkflowRunStatus::Pending, WorkflowRunEvent::Cancel).unwrap(),
            WorkflowRunStatus::Canceled
        );
    }
    #[test]
    fn test_running_pause() {
        assert_eq!(
            transition_workflow_run(WorkflowRunStatus::Running, WorkflowRunEvent::Pause).unwrap(),
            WorkflowRunStatus::Paused
        );
    }
    #[test]
    fn test_running_complete() {
        assert_eq!(
            transition_workflow_run(WorkflowRunStatus::Running, WorkflowRunEvent::Complete)
                .unwrap(),
            WorkflowRunStatus::Completed
        );
    }
    #[test]
    fn test_running_fail() {
        assert_eq!(
            transition_workflow_run(WorkflowRunStatus::Running, WorkflowRunEvent::Fail).unwrap(),
            WorkflowRunStatus::Failed
        );
    }
    #[test]
    fn test_running_timeout() {
        assert_eq!(
            transition_workflow_run(WorkflowRunStatus::Running, WorkflowRunEvent::Timeout).unwrap(),
            WorkflowRunStatus::TimedOut
        );
    }
    #[test]
    fn test_running_cancel() {
        assert_eq!(
            transition_workflow_run(WorkflowRunStatus::Running, WorkflowRunEvent::Cancel).unwrap(),
            WorkflowRunStatus::Canceled
        );
    }
    #[test]
    fn test_paused_resume() {
        assert_eq!(
            transition_workflow_run(WorkflowRunStatus::Paused, WorkflowRunEvent::Resume).unwrap(),
            WorkflowRunStatus::Running
        );
    }
    #[test]
    fn test_paused_cancel() {
        assert_eq!(
            transition_workflow_run(WorkflowRunStatus::Paused, WorkflowRunEvent::Cancel).unwrap(),
            WorkflowRunStatus::Canceled
        );
    }

    // Invalid transitions
    #[test]
    fn test_invalid_completed_start() {
        assert!(
            transition_workflow_run(WorkflowRunStatus::Completed, WorkflowRunEvent::Start).is_err()
        );
    }
    #[test]
    fn test_invalid_failed_resume() {
        assert!(
            transition_workflow_run(WorkflowRunStatus::Failed, WorkflowRunEvent::Resume).is_err()
        );
    }
    #[test]
    fn test_invalid_timed_out_pause() {
        assert!(
            transition_workflow_run(WorkflowRunStatus::TimedOut, WorkflowRunEvent::Pause).is_err()
        );
    }
    #[test]
    fn test_invalid_canceled_start() {
        assert!(
            transition_workflow_run(WorkflowRunStatus::Canceled, WorkflowRunEvent::Start).is_err()
        );
    }
    #[test]
    fn test_invalid_pending_pause() {
        assert!(
            transition_workflow_run(WorkflowRunStatus::Pending, WorkflowRunEvent::Pause).is_err()
        );
    }
    #[test]
    fn test_invalid_pending_resume() {
        assert!(
            transition_workflow_run(WorkflowRunStatus::Pending, WorkflowRunEvent::Resume).is_err()
        );
    }

    // can_transition tests
    #[test]
    fn test_can_transition_true() {
        assert!(can_transition_workflow_run(
            WorkflowRunStatus::Running,
            WorkflowRunEvent::Complete
        ));
    }
    #[test]
    fn test_can_transition_false() {
        assert!(!can_transition_workflow_run(
            WorkflowRunStatus::Completed,
            WorkflowRunEvent::Start
        ));
    }
    #[test]
    fn test_can_transition_paused_resume() {
        assert!(can_transition_workflow_run(
            WorkflowRunStatus::Paused,
            WorkflowRunEvent::Resume
        ));
    }

    // Terminal status tests
    #[test]
    fn test_terminal_completed() {
        assert!(is_terminal_workflow_run_status(
            WorkflowRunStatus::Completed
        ));
    }
    #[test]
    fn test_terminal_failed() {
        assert!(is_terminal_workflow_run_status(WorkflowRunStatus::Failed));
    }
    #[test]
    fn test_terminal_timed_out() {
        assert!(is_terminal_workflow_run_status(WorkflowRunStatus::TimedOut));
    }
    #[test]
    fn test_terminal_canceled() {
        assert!(is_terminal_workflow_run_status(WorkflowRunStatus::Canceled));
    }
    #[test]
    fn test_not_terminal_pending() {
        assert!(!is_terminal_workflow_run_status(WorkflowRunStatus::Pending));
    }
    #[test]
    fn test_not_terminal_running() {
        assert!(!is_terminal_workflow_run_status(WorkflowRunStatus::Running));
    }
    #[test]
    fn test_not_terminal_paused() {
        assert!(!is_terminal_workflow_run_status(WorkflowRunStatus::Paused));
    }

    // as_str tests
    #[test]
    fn test_status_as_str_pending() {
        assert_eq!(WorkflowRunStatus::Pending.as_str(), "pending");
    }
    #[test]
    fn test_status_as_str_running() {
        assert_eq!(WorkflowRunStatus::Running.as_str(), "running");
    }
    #[test]
    fn test_status_as_str_paused() {
        assert_eq!(WorkflowRunStatus::Paused.as_str(), "paused");
    }
    #[test]
    fn test_status_as_str_completed() {
        assert_eq!(WorkflowRunStatus::Completed.as_str(), "completed");
    }
    #[test]
    fn test_status_as_str_failed() {
        assert_eq!(WorkflowRunStatus::Failed.as_str(), "failed");
    }
    #[test]
    fn test_status_as_str_timed_out() {
        assert_eq!(WorkflowRunStatus::TimedOut.as_str(), "timed_out");
    }
    #[test]
    fn test_status_as_str_canceled() {
        assert_eq!(WorkflowRunStatus::Canceled.as_str(), "canceled");
    }
}
