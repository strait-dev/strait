# frozen_string_literal: true

module Strait
  module FSM
    STEP_RUN_PENDING = "pending"
    STEP_RUN_WAITING = "waiting"
    STEP_RUN_RUNNING = "running"
    STEP_RUN_COMPLETED = "completed"
    STEP_RUN_FAILED = "failed"
    STEP_RUN_SKIPPED = "skipped"
    STEP_RUN_CANCELED = "canceled"

    STEP_RUN_EVENT_WAIT = "WAIT"
    STEP_RUN_EVENT_START = "START"
    STEP_RUN_EVENT_COMPLETE = "COMPLETE"
    STEP_RUN_EVENT_FAIL = "FAIL"
    STEP_RUN_EVENT_SKIP = "SKIP"
    STEP_RUN_EVENT_CANCEL = "CANCEL"

    TERMINAL_STEP_RUN_STATUSES = [
      STEP_RUN_COMPLETED, STEP_RUN_FAILED,
      STEP_RUN_SKIPPED, STEP_RUN_CANCELED
    ].freeze

    STEP_RUN_TRANSITIONS = {
      [STEP_RUN_PENDING, STEP_RUN_EVENT_WAIT] => STEP_RUN_WAITING,
      [STEP_RUN_PENDING, STEP_RUN_EVENT_START] => STEP_RUN_RUNNING,
      [STEP_RUN_PENDING, STEP_RUN_EVENT_SKIP] => STEP_RUN_SKIPPED,
      [STEP_RUN_PENDING, STEP_RUN_EVENT_CANCEL] => STEP_RUN_CANCELED,
      [STEP_RUN_WAITING, STEP_RUN_EVENT_START] => STEP_RUN_RUNNING,
      [STEP_RUN_WAITING, STEP_RUN_EVENT_CANCEL] => STEP_RUN_CANCELED,
      [STEP_RUN_RUNNING, STEP_RUN_EVENT_COMPLETE] => STEP_RUN_COMPLETED,
      [STEP_RUN_RUNNING, STEP_RUN_EVENT_FAIL] => STEP_RUN_FAILED,
      [STEP_RUN_RUNNING, STEP_RUN_EVENT_CANCEL] => STEP_RUN_CANCELED,
    }.freeze

    def self.can_transition_step_run?(from, event)
      STEP_RUN_TRANSITIONS.key?([from, event])
    end

    def self.transition_step_run(from, event)
      to = STEP_RUN_TRANSITIONS[[from, event]]
      raise ArgumentError, "invalid step run transition: #{from} + #{event}" unless to
      to
    end

    def self.terminal_step_run_status?(status)
      TERMINAL_STEP_RUN_STATUSES.include?(status)
    end
  end
end
