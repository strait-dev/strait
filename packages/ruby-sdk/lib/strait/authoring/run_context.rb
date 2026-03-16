# frozen_string_literal: true

module Strait
  module Authoring
    # KV state store operations for a run.
    class RunContextState
      attr_reader :get, :set, :delete, :list

      def initialize(get:, set:, delete:, list:)
        @get = get
        @set = set
        @delete = delete
        @list = list
      end
    end

    # Context object passed to a job's run handler.
    class RunContext
      attr_accessor :run_id, :attempt, :checkpoint, :report_progress, :heartbeat,
                    :report_usage, :log_tool_call, :save_output, :state,
                    :stream_chunk, :wait_for_event, :spawn, :continue_run,
                    :annotate, :complete, :fail

      def initialize(
        run_id:,
        attempt: 1,
        checkpoint: nil,
        report_progress: nil,
        heartbeat: nil,
        report_usage: nil,
        log_tool_call: nil,
        save_output: nil,
        state: nil,
        stream_chunk: nil,
        wait_for_event: nil,
        spawn: nil,
        continue_run: nil,
        annotate: nil,
        complete: nil,
        fail: nil
      )
        @run_id = run_id
        @attempt = attempt
        @checkpoint = checkpoint
        @report_progress = report_progress
        @heartbeat = heartbeat
        @report_usage = report_usage
        @log_tool_call = log_tool_call
        @save_output = save_output
        @state = state
        @stream_chunk = stream_chunk
        @wait_for_event = wait_for_event
        @spawn = spawn
        @continue_run = continue_run
        @annotate = annotate
        @complete = complete
        @fail = fail
      end
    end
  end
end
