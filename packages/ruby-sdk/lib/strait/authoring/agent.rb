# frozen_string_literal: true

module Strait
  module Authoring
    # Agent-specific run context with iteration and cost tracking.
    class AgentRunContext < RunContext
      attr_accessor :iteration

      def initialize(max_cost_microusd: Float::INFINITY, **kwargs)
        super(**kwargs)
        @iteration = 0
        @accumulated_cost_microusd = 0
        @max_cost_microusd = max_cost_microusd
      end

      def accumulated_cost_microusd
        @accumulated_cost_microusd
      end

      def add_cost(cost)
        @accumulated_cost_microusd += cost
      end

      def budget_exceeded?
        @accumulated_cost_microusd >= @max_cost_microusd
      end
    end

    # Options for defining an agent.
    AgentOptions = Struct.new(
      :name, :slug, :endpoint_url, :project_id, :description, :tags,
      :max_iterations, :max_cost_microusd, :auto_checkpoint, :timeout_secs,
      :max_attempts, :retry_strategy, :run, :on_success, :on_failure, :on_start,
      keyword_init: true
    ) do
      def initialize(**)
        super
        self.auto_checkpoint = true if auto_checkpoint.nil?
      end
    end

    # Creates a job definition with agent conventions.
    def self.define_agent(opts)
      max_cost = opts.max_cost_microusd || Float::INFINITY
      auto_checkpoint = opts.auto_checkpoint
      user_run = opts.run

      agent_run = ->(payload, ctx) {
        agent_ctx = AgentRunContext.new(
          run_id: ctx.run_id,
          attempt: ctx.attempt,
          report_progress: ctx.report_progress,
          heartbeat: ctx.heartbeat,
          log_tool_call: ctx.log_tool_call,
          save_output: ctx.save_output,
          state: ctx.state,
          stream_chunk: ctx.stream_chunk,
          wait_for_event: ctx.wait_for_event,
          spawn: ctx.spawn,
          continue_run: ctx.continue_run,
          annotate: ctx.annotate,
          complete: ctx.complete,
          fail: ctx.fail,
          max_cost_microusd: max_cost
        )

        # Wrap report_usage to track cost
        if ctx.report_usage
          original_report_usage = ctx.report_usage
          agent_ctx.report_usage = ->(**kwargs) {
            if kwargs[:cost_microusd]
              agent_ctx.add_cost(kwargs[:cost_microusd])
            end
            original_report_usage.call(**kwargs)
          }
        end

        # Wrap checkpoint to track iterations
        original_checkpoint = ctx.checkpoint
        agent_ctx.checkpoint = ->(state) {
          agent_ctx.iteration += 1
          original_checkpoint&.call(state) if auto_checkpoint
        }

        user_run&.call(payload, agent_ctx)
      }

      tags = (opts.tags || {}).merge("strait.kind" => "agent")

      job_opts = JobOptions.new(
        name: opts.name,
        slug: opts.slug,
        endpoint_url: opts.endpoint_url,
        project_id: opts.project_id,
        description: opts.description,
        tags: tags,
        timeout_secs: opts.timeout_secs || 600,
        max_attempts: opts.max_attempts || 5,
        retry_strategy: opts.retry_strategy || "exponential",
        run: agent_run,
        on_success: opts.on_success,
        on_failure: opts.on_failure,
        on_start: opts.on_start
      )

      JobDefinition.new(job_opts)
    end
  end
end
