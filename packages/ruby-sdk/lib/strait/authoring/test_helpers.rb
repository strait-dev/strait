# frozen_string_literal: true

module Strait
  module Authoring
    # Captures all operations on a test RunContext.
    class TestRunRecord
      attr_accessor :checkpoints, :logs, :usage_reports, :tool_calls, :outputs,
                    :progress_updates, :state_store, :stream_chunks, :heartbeats,
                    :spawns, :events, :annotations, :continuations,
                    :completed, :failed, :fail_error, :result

      def initialize
        @checkpoints = []
        @logs = []
        @usage_reports = []
        @tool_calls = []
        @outputs = []
        @progress_updates = []
        @state_store = {}
        @stream_chunks = []
        @heartbeats = 0
        @spawns = []
        @events = []
        @annotations = []
        @continuations = []
        @completed = false
        @failed = false
        @fail_error = nil
        @result = nil
      end
    end

    # Creates an in-memory RunContext and TestRunRecord for testing.
    #
    # @param run_id [String] The run identifier (default "test-run")
    # @param attempt [Integer] The attempt number (default 1)
    # @return [Array(RunContext, TestRunRecord)]
    def self.create_test_context(run_id: "test-run", attempt: 1)
      record = TestRunRecord.new

      ctx = RunContext.new(
        run_id: run_id,
        attempt: attempt,

        checkpoint: ->(state) { record.checkpoints << state },

        report_progress: ->(percent, message = nil) {
          record.progress_updates << { "percent" => percent, "message" => message }
        },

        heartbeat: -> { record.heartbeats += 1 },

        report_usage: ->(**kwargs) { record.usage_reports << kwargs },

        log_tool_call: ->(**kwargs) { record.tool_calls << kwargs },

        save_output: ->(key, value, _schema = nil) {
          record.outputs << { "key" => key, "value" => value }
        },

        state: RunContextState.new(
          get: ->(key) { record.state_store[key] },
          set: ->(key, value) { record.state_store[key] = value },
          delete: ->(key) { record.state_store.delete(key) },
          list: -> {
            record.state_store.map { |k, v| { "key" => k, "value" => v } }
          }
        ),

        stream_chunk: ->(chunk, stream_id: nil, done: nil) {
          entry = { "chunk" => chunk }
          entry["stream_id"] = stream_id if stream_id
          entry["done"] = done unless done.nil?
          record.stream_chunks << entry
        },

        wait_for_event: ->(event_key, timeout_secs: nil, notify_url: nil) {
          record.events << { "event_key" => event_key, "timeout_secs" => timeout_secs, "notify_url" => notify_url }
          { "status" => "waiting", "event_key" => event_key, "trigger_id" => "trigger_test" }
        },

        spawn: ->(job_slug:, project_id:, payload: nil, priority: nil) {
          record.spawns << { "job_slug" => job_slug, "project_id" => project_id, "payload" => payload }
          { "id" => "spawn_#{record.spawns.length}" }
        },

        continue_run: ->(payload = nil) {
          record.continuations << { "payload" => payload }
          { "id" => "continue_#{record.continuations.length}" }
        },

        annotate: ->(annotations) { record.annotations << annotations },

        complete: ->(result = nil) {
          record.completed = true
          record.result = result
        },

        fail: ->(error) {
          record.failed = true
          record.fail_error = error
        }
      )

      [ctx, record]
    end
  end
end
