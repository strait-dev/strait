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
