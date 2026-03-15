# frozen_string_literal: true

module Strait
  module Operations
    # Service for SDK-side run lifecycle operations (used by worker SDKs).
    class SDKRunsService < BaseService
      # Annotate a run with metadata.
      # POST /sdk/v1/runs/{runID}/annotate
      def annotate_run(run_id, body)
        _request(:post, "/sdk/v1/runs/{runID}/annotate", path_params: { "runID" => run_id }, body: body)
      end

      # Create a checkpoint for a run.
      # POST /sdk/v1/runs/{runID}/checkpoint
      def checkpoint_run(run_id, body)
        _request(:post, "/sdk/v1/runs/{runID}/checkpoint", path_params: { "runID" => run_id }, body: body)
      end

      # Mark a run as complete.
      # POST /sdk/v1/runs/{runID}/complete
      def complete_run(run_id, body)
        _request(:post, "/sdk/v1/runs/{runID}/complete", path_params: { "runID" => run_id }, body: body)
      end

      # Continue a suspended run.
      # POST /sdk/v1/runs/{runID}/continue
      def continue_run(run_id, body)
        _request(:post, "/sdk/v1/runs/{runID}/continue", path_params: { "runID" => run_id }, body: body)
      end

      # Mark a run as failed.
      # POST /sdk/v1/runs/{runID}/fail
      def fail_run(run_id, body)
        _request(:post, "/sdk/v1/runs/{runID}/fail", path_params: { "runID" => run_id }, body: body)
      end

      # Send a heartbeat for a run.
      # POST /sdk/v1/runs/{runID}/heartbeat
      def heartbeat_run(run_id)
        _request(:post, "/sdk/v1/runs/{runID}/heartbeat", path_params: { "runID" => run_id })
      end

      # Append a log entry to a run.
      # POST /sdk/v1/runs/{runID}/log
      def log_run(run_id, body)
        _request(:post, "/sdk/v1/runs/{runID}/log", path_params: { "runID" => run_id }, body: body)
      end

      # Record an output for a run.
      # POST /sdk/v1/runs/{runID}/output
      def output_run(run_id, body)
        _request(:post, "/sdk/v1/runs/{runID}/output", path_params: { "runID" => run_id }, body: body)
      end

      # Report progress for a run.
      # POST /sdk/v1/runs/{runID}/progress
      def progress_run(run_id, body)
        _request(:post, "/sdk/v1/runs/{runID}/progress", path_params: { "runID" => run_id }, body: body)
      end

      # Spawn a child run.
      # POST /sdk/v1/runs/{runID}/spawn
      def spawn_run(run_id, body)
        _request(:post, "/sdk/v1/runs/{runID}/spawn", path_params: { "runID" => run_id }, body: body)
      end

      # Record a tool call for a run.
      # POST /sdk/v1/runs/{runID}/tool-call
      def tool_call_run(run_id, body)
        _request(:post, "/sdk/v1/runs/{runID}/tool-call", path_params: { "runID" => run_id }, body: body)
      end

      # Report resource usage for a run.
      # POST /sdk/v1/runs/{runID}/usage
      def usage_run(run_id, body)
        _request(:post, "/sdk/v1/runs/{runID}/usage", path_params: { "runID" => run_id }, body: body)
      end

      # Wait for an event during a run.
      # POST /sdk/v1/runs/{runID}/wait-for-event
      def wait_for_event_run(run_id, body)
        _request(:post, "/sdk/v1/runs/{runID}/wait-for-event", path_params: { "runID" => run_id }, body: body)
      end
    end
  end
end
