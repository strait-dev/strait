use crate::errors::StraitError;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum RunStatus {
    Delayed,
    Queued,
    Dequeued,
    Executing,
    Waiting,
    Completed,
    Failed,
    TimedOut,
    Crashed,
    SystemFailed,
    Canceled,
    Expired,
    DeadLetter,
    ReplayStaged,
}

impl RunStatus {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Delayed => "delayed",
            Self::Queued => "queued",
            Self::Dequeued => "dequeued",
            Self::Executing => "executing",
            Self::Waiting => "waiting",
            Self::Completed => "completed",
            Self::Failed => "failed",
            Self::TimedOut => "timed_out",
            Self::Crashed => "crashed",
            Self::SystemFailed => "system_failed",
            Self::Canceled => "canceled",
            Self::Expired => "expired",
            Self::DeadLetter => "dead_letter",
            Self::ReplayStaged => "replay_staged",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum RunEvent {
    Enqueue,
    Dequeue,
    Execute,
    Complete,
    Fail,
    Timeout,
    Crash,
    SystemFail,
    Cancel,
    Expire,
    Wait,
    Requeue,
    DeadLetter,
    Replay,
}

pub fn can_transition_run(from: RunStatus, event: RunEvent) -> bool {
    transition_run(from, event).is_ok()
}

pub fn transition_run(from: RunStatus, event: RunEvent) -> Result<RunStatus, StraitError> {
    use RunStatus::*;
    use RunEvent::*;

    let to = match (from, event) {
        (Delayed, Enqueue) => Queued,
        (Delayed, Cancel) => Canceled,
        (Delayed, Expire) => Expired,
        (Queued, Dequeue) => Dequeued,
        (Queued, Cancel) => Canceled,
        (Queued, Expire) => Expired,
        (Dequeued, Execute) => Executing,
        (Dequeued, Cancel) => Canceled,
        (Dequeued, Requeue) => Queued,
        (Executing, Complete) => Completed,
        (Executing, Fail) => Failed,
        (Executing, Timeout) => TimedOut,
        (Executing, Crash) => Crashed,
        (Executing, SystemFail) => SystemFailed,
        (Executing, Cancel) => Canceled,
        (Executing, Wait) => Waiting,
        (Waiting, Execute) => Executing,
        (Waiting, Cancel) => Canceled,
        (Waiting, Timeout) => TimedOut,
        (Failed, Requeue) => Queued,
        (Failed, RunEvent::DeadLetter) => RunStatus::DeadLetter,
        (Failed, Replay) => ReplayStaged,
        (TimedOut, Replay) => ReplayStaged,
        (Crashed, Replay) => ReplayStaged,
        (SystemFailed, Replay) => ReplayStaged,
        (RunStatus::DeadLetter, Replay) => ReplayStaged,
        (ReplayStaged, Enqueue) => Queued,
        _ => {
            return Err(StraitError::Validation {
                message: format!("invalid run transition: {:?} + {:?}", from, event),
                issues: vec![],
            });
        }
    };
    Ok(to)
}

pub fn is_terminal_run_status(status: RunStatus) -> bool {
    matches!(
        status,
        RunStatus::Completed
            | RunStatus::Failed
            | RunStatus::TimedOut
            | RunStatus::Crashed
            | RunStatus::SystemFailed
            | RunStatus::Canceled
            | RunStatus::Expired
    )
}
