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
    use RunEvent::*;
    use RunStatus::*;

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

#[cfg(test)]
mod tests {
    use super::*;

    // Transition tests
    #[test]
    fn test_delayed_enqueue() {
        assert_eq!(
            transition_run(RunStatus::Delayed, RunEvent::Enqueue).unwrap(),
            RunStatus::Queued
        );
    }
    #[test]
    fn test_delayed_cancel() {
        assert_eq!(
            transition_run(RunStatus::Delayed, RunEvent::Cancel).unwrap(),
            RunStatus::Canceled
        );
    }
    #[test]
    fn test_delayed_expire() {
        assert_eq!(
            transition_run(RunStatus::Delayed, RunEvent::Expire).unwrap(),
            RunStatus::Expired
        );
    }
    #[test]
    fn test_queued_dequeue() {
        assert_eq!(
            transition_run(RunStatus::Queued, RunEvent::Dequeue).unwrap(),
            RunStatus::Dequeued
        );
    }
    #[test]
    fn test_queued_cancel() {
        assert_eq!(
            transition_run(RunStatus::Queued, RunEvent::Cancel).unwrap(),
            RunStatus::Canceled
        );
    }
    #[test]
    fn test_queued_expire() {
        assert_eq!(
            transition_run(RunStatus::Queued, RunEvent::Expire).unwrap(),
            RunStatus::Expired
        );
    }
    #[test]
    fn test_dequeued_execute() {
        assert_eq!(
            transition_run(RunStatus::Dequeued, RunEvent::Execute).unwrap(),
            RunStatus::Executing
        );
    }
    #[test]
    fn test_dequeued_cancel() {
        assert_eq!(
            transition_run(RunStatus::Dequeued, RunEvent::Cancel).unwrap(),
            RunStatus::Canceled
        );
    }
    #[test]
    fn test_dequeued_requeue() {
        assert_eq!(
            transition_run(RunStatus::Dequeued, RunEvent::Requeue).unwrap(),
            RunStatus::Queued
        );
    }
    #[test]
    fn test_executing_complete() {
        assert_eq!(
            transition_run(RunStatus::Executing, RunEvent::Complete).unwrap(),
            RunStatus::Completed
        );
    }
    #[test]
    fn test_executing_fail() {
        assert_eq!(
            transition_run(RunStatus::Executing, RunEvent::Fail).unwrap(),
            RunStatus::Failed
        );
    }
    #[test]
    fn test_executing_timeout() {
        assert_eq!(
            transition_run(RunStatus::Executing, RunEvent::Timeout).unwrap(),
            RunStatus::TimedOut
        );
    }
    #[test]
    fn test_executing_crash() {
        assert_eq!(
            transition_run(RunStatus::Executing, RunEvent::Crash).unwrap(),
            RunStatus::Crashed
        );
    }
    #[test]
    fn test_executing_system_fail() {
        assert_eq!(
            transition_run(RunStatus::Executing, RunEvent::SystemFail).unwrap(),
            RunStatus::SystemFailed
        );
    }
    #[test]
    fn test_executing_cancel() {
        assert_eq!(
            transition_run(RunStatus::Executing, RunEvent::Cancel).unwrap(),
            RunStatus::Canceled
        );
    }
    #[test]
    fn test_executing_wait() {
        assert_eq!(
            transition_run(RunStatus::Executing, RunEvent::Wait).unwrap(),
            RunStatus::Waiting
        );
    }
    #[test]
    fn test_waiting_execute() {
        assert_eq!(
            transition_run(RunStatus::Waiting, RunEvent::Execute).unwrap(),
            RunStatus::Executing
        );
    }
    #[test]
    fn test_waiting_cancel() {
        assert_eq!(
            transition_run(RunStatus::Waiting, RunEvent::Cancel).unwrap(),
            RunStatus::Canceled
        );
    }
    #[test]
    fn test_waiting_timeout() {
        assert_eq!(
            transition_run(RunStatus::Waiting, RunEvent::Timeout).unwrap(),
            RunStatus::TimedOut
        );
    }
    #[test]
    fn test_failed_requeue() {
        assert_eq!(
            transition_run(RunStatus::Failed, RunEvent::Requeue).unwrap(),
            RunStatus::Queued
        );
    }
    #[test]
    fn test_failed_dead_letter() {
        assert_eq!(
            transition_run(RunStatus::Failed, RunEvent::DeadLetter).unwrap(),
            RunStatus::DeadLetter
        );
    }
    #[test]
    fn test_failed_replay() {
        assert_eq!(
            transition_run(RunStatus::Failed, RunEvent::Replay).unwrap(),
            RunStatus::ReplayStaged
        );
    }
    #[test]
    fn test_timed_out_replay() {
        assert_eq!(
            transition_run(RunStatus::TimedOut, RunEvent::Replay).unwrap(),
            RunStatus::ReplayStaged
        );
    }
    #[test]
    fn test_crashed_replay() {
        assert_eq!(
            transition_run(RunStatus::Crashed, RunEvent::Replay).unwrap(),
            RunStatus::ReplayStaged
        );
    }
    #[test]
    fn test_system_failed_replay() {
        assert_eq!(
            transition_run(RunStatus::SystemFailed, RunEvent::Replay).unwrap(),
            RunStatus::ReplayStaged
        );
    }
    #[test]
    fn test_dead_letter_replay() {
        assert_eq!(
            transition_run(RunStatus::DeadLetter, RunEvent::Replay).unwrap(),
            RunStatus::ReplayStaged
        );
    }
    #[test]
    fn test_replay_staged_enqueue() {
        assert_eq!(
            transition_run(RunStatus::ReplayStaged, RunEvent::Enqueue).unwrap(),
            RunStatus::Queued
        );
    }

    // Invalid transitions
    #[test]
    fn test_invalid_completed_execute() {
        assert!(transition_run(RunStatus::Completed, RunEvent::Execute).is_err());
    }
    #[test]
    fn test_invalid_canceled_enqueue() {
        assert!(transition_run(RunStatus::Canceled, RunEvent::Enqueue).is_err());
    }
    #[test]
    fn test_invalid_expired_execute() {
        assert!(transition_run(RunStatus::Expired, RunEvent::Execute).is_err());
    }

    // can_transition tests
    #[test]
    fn test_can_transition_true() {
        assert!(can_transition_run(RunStatus::Executing, RunEvent::Complete));
    }
    #[test]
    fn test_can_transition_false() {
        assert!(!can_transition_run(RunStatus::Completed, RunEvent::Execute));
    }

    // Terminal status tests
    #[test]
    fn test_terminal_completed() {
        assert!(is_terminal_run_status(RunStatus::Completed));
    }
    #[test]
    fn test_terminal_failed() {
        assert!(is_terminal_run_status(RunStatus::Failed));
    }
    #[test]
    fn test_terminal_timed_out() {
        assert!(is_terminal_run_status(RunStatus::TimedOut));
    }
    #[test]
    fn test_terminal_crashed() {
        assert!(is_terminal_run_status(RunStatus::Crashed));
    }
    #[test]
    fn test_terminal_system_failed() {
        assert!(is_terminal_run_status(RunStatus::SystemFailed));
    }
    #[test]
    fn test_terminal_canceled() {
        assert!(is_terminal_run_status(RunStatus::Canceled));
    }
    #[test]
    fn test_terminal_expired() {
        assert!(is_terminal_run_status(RunStatus::Expired));
    }
    #[test]
    fn test_not_terminal_executing() {
        assert!(!is_terminal_run_status(RunStatus::Executing));
    }
    #[test]
    fn test_not_terminal_queued() {
        assert!(!is_terminal_run_status(RunStatus::Queued));
    }
    #[test]
    fn test_not_terminal_delayed() {
        assert!(!is_terminal_run_status(RunStatus::Delayed));
    }
    #[test]
    fn test_not_terminal_dequeued() {
        assert!(!is_terminal_run_status(RunStatus::Dequeued));
    }
    #[test]
    fn test_not_terminal_waiting() {
        assert!(!is_terminal_run_status(RunStatus::Waiting));
    }
    #[test]
    fn test_not_terminal_dead_letter() {
        assert!(!is_terminal_run_status(RunStatus::DeadLetter));
    }
    #[test]
    fn test_not_terminal_replay_staged() {
        assert!(!is_terminal_run_status(RunStatus::ReplayStaged));
    }

    // as_str tests
    #[test]
    fn test_status_as_str_delayed() {
        assert_eq!(RunStatus::Delayed.as_str(), "delayed");
    }
    #[test]
    fn test_status_as_str_queued() {
        assert_eq!(RunStatus::Queued.as_str(), "queued");
    }
    #[test]
    fn test_status_as_str_dequeued() {
        assert_eq!(RunStatus::Dequeued.as_str(), "dequeued");
    }
    #[test]
    fn test_status_as_str_executing() {
        assert_eq!(RunStatus::Executing.as_str(), "executing");
    }
    #[test]
    fn test_status_as_str_completed() {
        assert_eq!(RunStatus::Completed.as_str(), "completed");
    }
    #[test]
    fn test_status_as_str_failed() {
        assert_eq!(RunStatus::Failed.as_str(), "failed");
    }
    #[test]
    fn test_status_as_str_timed_out() {
        assert_eq!(RunStatus::TimedOut.as_str(), "timed_out");
    }
    #[test]
    fn test_status_as_str_crashed() {
        assert_eq!(RunStatus::Crashed.as_str(), "crashed");
    }
    #[test]
    fn test_status_as_str_system_failed() {
        assert_eq!(RunStatus::SystemFailed.as_str(), "system_failed");
    }
    #[test]
    fn test_status_as_str_canceled() {
        assert_eq!(RunStatus::Canceled.as_str(), "canceled");
    }
    #[test]
    fn test_status_as_str_expired() {
        assert_eq!(RunStatus::Expired.as_str(), "expired");
    }
    #[test]
    fn test_status_as_str_dead_letter() {
        assert_eq!(RunStatus::DeadLetter.as_str(), "dead_letter");
    }
    #[test]
    fn test_status_as_str_replay_staged() {
        assert_eq!(RunStatus::ReplayStaged.as_str(), "replay_staged");
    }
}
