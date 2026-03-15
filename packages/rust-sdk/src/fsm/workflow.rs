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
    use WorkflowRunStatus::*;
    use WorkflowRunEvent::*;

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
