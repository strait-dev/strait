# frozen_string_literal: true

module Strait
  module Authoring
    # Step type constants
    STEP_TYPE_JOB = "job"
    STEP_TYPE_APPROVAL = "approval"
    STEP_TYPE_SUB_WORKFLOW = "sub_workflow"
    STEP_TYPE_WAIT_FOR_EVENT = "wait_for_event"
    STEP_TYPE_SLEEP = "sleep"

    # On failure action constants
    ON_FAILURE_FAIL_WORKFLOW = "fail_workflow"
    ON_FAILURE_SKIP_DEPENDENTS = "skip_dependents"
    ON_FAILURE_CONTINUE = "continue"

    # Resource class constants
    RESOURCE_CLASS_SMALL = "small"
    RESOURCE_CLASS_MEDIUM = "medium"
    RESOURCE_CLASS_LARGE = "large"

    # Retry backoff constants
    RETRY_BACKOFF_EXPONENTIAL = "exponential"
    RETRY_BACKOFF_FIXED = "fixed"

    BaseStepOptions = Struct.new(
      :depends_on, :condition, :on_failure, :payload,
      :retry_max_attempts, :retry_backoff, :retry_initial_delay_secs,
      :retry_max_delay_secs, :timeout_secs_override, :output_transform,
      :concurrency_key, :resource_class,
      keyword_init: true
    ) do
      def initialize(**kwargs)
        super
        self.depends_on ||= []
      end
    end

    class JobStep
      attr_reader :ref, :job_id, :options

      def initialize(ref, job_id, **opts)
        @ref = ref
        @job_id = job_id
        @options = BaseStepOptions.new(**opts)
      end

      def type = STEP_TYPE_JOB
      def depends_on = options.depends_on

      def to_api
        h = { "ref" => ref, "type" => type, "job_id" => job_id }
        h["depends_on"] = options.depends_on unless options.depends_on.empty?
        h["condition"] = options.condition if options.condition
        h["on_failure"] = options.on_failure if options.on_failure
        h["payload"] = options.payload if options.payload
        h["retry_max_attempts"] = options.retry_max_attempts if options.retry_max_attempts
        h["retry_backoff"] = options.retry_backoff if options.retry_backoff
        h["retry_initial_delay_secs"] = options.retry_initial_delay_secs if options.retry_initial_delay_secs
        h["retry_max_delay_secs"] = options.retry_max_delay_secs if options.retry_max_delay_secs
        h["timeout_secs_override"] = options.timeout_secs_override if options.timeout_secs_override
        h["output_transform"] = options.output_transform if options.output_transform
        h["concurrency_key"] = options.concurrency_key if options.concurrency_key
        h["resource_class"] = options.resource_class if options.resource_class
        h
      end
    end

    class ApprovalStep
      attr_reader :ref, :approval_timeout_secs, :approvers, :options

      def initialize(ref, approval_timeout_secs: nil, approvers: nil, **opts)
        @ref = ref
        @approval_timeout_secs = approval_timeout_secs
        @approvers = approvers
        @options = BaseStepOptions.new(**opts)
      end

      def type = STEP_TYPE_APPROVAL
      def depends_on = options.depends_on

      def to_api
        h = { "ref" => ref, "type" => type }
        h["approval_timeout_secs"] = approval_timeout_secs if approval_timeout_secs
        h["approvers"] = approvers if approvers
        h["depends_on"] = options.depends_on unless options.depends_on.empty?
        h["condition"] = options.condition if options.condition
        h["on_failure"] = options.on_failure if options.on_failure
        h
      end
    end

    class SubWorkflowStep
      attr_reader :ref, :sub_workflow_id, :max_nesting_depth, :options

      def initialize(ref, sub_workflow_id, max_nesting_depth: nil, **opts)
        @ref = ref
        @sub_workflow_id = sub_workflow_id
        @max_nesting_depth = max_nesting_depth
        @options = BaseStepOptions.new(**opts)
      end

      def type = STEP_TYPE_SUB_WORKFLOW
      def depends_on = options.depends_on

      def to_api
        h = { "ref" => ref, "type" => type, "sub_workflow_id" => sub_workflow_id }
        h["max_nesting_depth"] = max_nesting_depth if max_nesting_depth
        h["depends_on"] = options.depends_on unless options.depends_on.empty?
        h["condition"] = options.condition if options.condition
        h["on_failure"] = options.on_failure if options.on_failure
        h["payload"] = options.payload if options.payload
        h
      end
    end

    class WaitForEventStep
      attr_reader :ref, :event_key, :event_timeout_secs, :event_notify_url, :options

      def initialize(ref, event_key, event_timeout_secs: nil, event_notify_url: nil, **opts)
        @ref = ref
        @event_key = event_key
        @event_timeout_secs = event_timeout_secs
        @event_notify_url = event_notify_url
        @options = BaseStepOptions.new(**opts)
      end

      def type = STEP_TYPE_WAIT_FOR_EVENT
      def depends_on = options.depends_on

      def to_api
        h = { "ref" => ref, "type" => type, "event_key" => event_key }
        h["event_timeout_secs"] = event_timeout_secs if event_timeout_secs
        h["event_notify_url"] = event_notify_url if event_notify_url
        h["depends_on"] = options.depends_on unless options.depends_on.empty?
        h["condition"] = options.condition if options.condition
        h["on_failure"] = options.on_failure if options.on_failure
        h
      end
    end

    class SleepStep
      attr_reader :ref, :sleep_duration_secs, :options

      def initialize(ref, sleep_duration_secs, **opts)
        @ref = ref
        @sleep_duration_secs = sleep_duration_secs
        @options = BaseStepOptions.new(**opts)
      end

      def type = STEP_TYPE_SLEEP
      def depends_on = options.depends_on

      def to_api
        h = { "ref" => ref, "type" => type, "sleep_duration_secs" => sleep_duration_secs }
        h["depends_on"] = options.depends_on unless options.depends_on.empty?
        h["condition"] = options.condition if options.condition
        h["on_failure"] = options.on_failure if options.on_failure
        h
      end
    end

    # Builder functions
    def self.job_step(ref, job_id, **opts)
      JobStep.new(ref, job_id, **opts)
    end

    def self.approval_step(ref, **opts)
      ApprovalStep.new(ref, **opts)
    end

    def self.sub_workflow_step(ref, sub_workflow_id, **opts)
      SubWorkflowStep.new(ref, sub_workflow_id, **opts)
    end

    def self.wait_for_event_step(ref, event_key, **opts)
      WaitForEventStep.new(ref, event_key, **opts)
    end

    def self.sleep_step(ref, duration_secs, **opts)
      SleepStep.new(ref, duration_secs, **opts)
    end

    # Creates a job step with LLM-tuned defaults.
    def self.ai_step(ref, job_id, **opts)
      JobStep.new(
        ref, job_id,
        depends_on: opts.fetch(:depends_on, []),
        condition: opts[:condition],
        on_failure: opts[:on_failure],
        payload: opts[:payload],
        retry_max_attempts: opts.fetch(:retry_max_attempts, 5),
        retry_backoff: opts.fetch(:retry_backoff, RETRY_BACKOFF_EXPONENTIAL),
        retry_initial_delay_secs: opts.fetch(:retry_initial_delay_secs, 2),
        retry_max_delay_secs: opts.fetch(:retry_max_delay_secs, 120),
        timeout_secs_override: opts.fetch(:timeout_secs_override, 600),
        output_transform: opts[:output_transform],
        concurrency_key: opts[:concurrency_key],
        resource_class: opts.fetch(:resource_class, RESOURCE_CLASS_LARGE)
      )
    end
  end
end
