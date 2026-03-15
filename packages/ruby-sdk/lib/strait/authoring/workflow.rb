# frozen_string_literal: true

module Strait
  module Authoring
    WorkflowOptions = Struct.new(
      :name, :slug, :steps, :project_id,
      :description, :tags, :environment_id,
      :max_concurrent_runs, :max_parallel_steps, :timeout_secs,
      :max_attempts, :retry_strategy,
      :cron, :timezone,
      :webhook_url, :webhook_secret,
      :run, :on_success, :on_failure,
      keyword_init: true
    )

    TriggerWorkflowInput = Struct.new(
      :workflow_id, :payload, :idempotency_key, :priority,
      :dry_run, :metadata, :step_overrides,
      keyword_init: true
    )

    class WorkflowDefinition
      attr_reader :kind, :slug, :run, :on_success, :on_failure
      attr_accessor :last_registered_workflow_id

      def initialize(opts)
        @kind = "workflow"
        @slug = opts.slug
        @opts = opts
        @run = opts.run
        @on_success = opts.on_success
        @on_failure = opts.on_failure
        @last_registered_workflow_id = nil
      end

      def to_registration_body(project_id = nil)
        pid = project_id || @opts.project_id
        body = {}
        body["name"] = @opts.name if @opts.name
        body["slug"] = @opts.slug if @opts.slug
        body["project_id"] = pid if pid
        body["description"] = @opts.description if @opts.description
        body["tags"] = @opts.tags if @opts.tags
        body["environment_id"] = @opts.environment_id if @opts.environment_id
        body["max_concurrent_runs"] = @opts.max_concurrent_runs if @opts.max_concurrent_runs
        body["max_parallel_steps"] = @opts.max_parallel_steps if @opts.max_parallel_steps
        body["timeout_secs"] = @opts.timeout_secs if @opts.timeout_secs
        body["max_attempts"] = @opts.max_attempts if @opts.max_attempts
        body["retry_strategy"] = @opts.retry_strategy if @opts.retry_strategy
        body["cron"] = @opts.cron if @opts.cron
        body["timezone"] = @opts.timezone if @opts.timezone
        body["webhook_url"] = @opts.webhook_url if @opts.webhook_url
        body["webhook_secret"] = @opts.webhook_secret if @opts.webhook_secret

        if @opts.steps
          Strait::Authoring.validate_dag(@opts.steps)
          body["steps"] = @opts.steps.map(&:to_api)
        end

        body
      end

      def register(client, project_id: nil)
        body = to_registration_body(project_id)
        result = client.workflows.create(body)
        @last_registered_workflow_id = result["id"] if result.is_a?(Hash)
        result
      end

      def trigger(client, input)
        wf_id = input.workflow_id || @last_registered_workflow_id
        raise ArgumentError, "workflow_id is required" unless wf_id

        body = {}
        body["payload"] = input.payload if input.payload
        body["idempotency_key"] = input.idempotency_key if input.idempotency_key
        body["priority"] = input.priority if input.priority
        body["dry_run"] = input.dry_run unless input.dry_run.nil?
        body["metadata"] = input.metadata if input.metadata
        body["step_overrides"] = input.step_overrides if input.step_overrides
        client.workflows.trigger(wf_id, body)
      end
    end

    def self.define_workflow(**opts)
      WorkflowDefinition.new(WorkflowOptions.new(**opts))
    end
  end
end
