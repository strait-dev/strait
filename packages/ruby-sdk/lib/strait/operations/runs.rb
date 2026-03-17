# frozen_string_literal: true

module Strait
  module Operations
    # Service for run management, replay, debugging, and bulk operations.
    class RunsService < BaseService
      # List all runs.
      # GET /v1/runs
      def list(query: nil)
        _request(:get, "/v1/runs", query: query)
      end

      # Get a run by ID.
      # GET /v1/runs/{runID}
      def get(run_id)
        _request(:get, "/v1/runs/{runID}", path_params: { "runID" => run_id })
      end

      # Delete a run.
      # DELETE /v1/runs/{runID}
      def delete(run_id)
        _request(:delete, "/v1/runs/{runID}", path_params: { "runID" => run_id })
        nil
      end

      # List checkpoints for a run.
      # GET /v1/runs/{runID}/checkpoints
      def list_checkpoints(run_id)
        _request(:get, "/v1/runs/{runID}/checkpoints", path_params: { "runID" => run_id })
      end

      # Get child runs.
      # GET /v1/runs/{runID}/children
      def get_children(run_id, query: nil)
        _request(:get, "/v1/runs/{runID}/children", path_params: { "runID" => run_id }, query: query)
      end

      # Start a debug session for a run.
      # POST /v1/runs/{runID}/debug
      def debug(run_id, body)
        _request(:post, "/v1/runs/{runID}/debug", path_params: { "runID" => run_id }, body: body)
      end

      # Get the debug bundle for a run.
      # GET /v1/runs/{runID}/debug-bundle
      def get_debug_bundle(run_id)
        _request(:get, "/v1/runs/{runID}/debug-bundle", path_params: { "runID" => run_id })
      end

      # List dependency status for a run.
      # GET /v1/runs/{runID}/dependency-status
      def list_dependency_status(run_id)
        _request(:get, "/v1/runs/{runID}/dependency-status", path_params: { "runID" => run_id })
      end

      # Replay a run from the dead-letter queue.
      # POST /v1/runs/{runID}/dlq-replay
      def dlq_replay(run_id, body)
        _request(:post, "/v1/runs/{runID}/dlq-replay", path_params: { "runID" => run_id }, body: body)
      end

      # List events for a run.
      # GET /v1/runs/{runID}/events
      def list_events(run_id, query: nil)
        _request(:get, "/v1/runs/{runID}/events", path_params: { "runID" => run_id }, query: query)
      end

      # Delete the idempotency key for a run.
      # DELETE /v1/runs/{runID}/idempotency-key
      def delete_idempotency_key(run_id)
        _request(:delete, "/v1/runs/{runID}/idempotency-key", path_params: { "runID" => run_id })
        nil
      end

      # Get run lineage (parent/child graph).
      # GET /v1/runs/{runID}/lineage
      def get_lineage(run_id)
        _request(:get, "/v1/runs/{runID}/lineage", path_params: { "runID" => run_id })
      end

      # List outputs for a run.
      # GET /v1/runs/{runID}/outputs
      def list_outputs(run_id)
        _request(:get, "/v1/runs/{runID}/outputs", path_params: { "runID" => run_id })
      end

      # Replay a run.
      # POST /v1/runs/{runID}/replay
      def replay(run_id, body)
        _request(:post, "/v1/runs/{runID}/replay", path_params: { "runID" => run_id }, body: body)
      end

      # Reschedule a run.
      # POST /v1/runs/{runID}/reschedule
      def reschedule(run_id, body)
        _request(:post, "/v1/runs/{runID}/reschedule", path_params: { "runID" => run_id }, body: body)
      end

      # List tool calls made by a run.
      # GET /v1/runs/{runID}/tool-calls
      def list_tool_calls(run_id)
        _request(:get, "/v1/runs/{runID}/tool-calls", path_params: { "runID" => run_id })
      end

      # Get resource usage for a run.
      # GET /v1/runs/{runID}/usage
      def get_usage(run_id)
        _request(:get, "/v1/runs/{runID}/usage", path_params: { "runID" => run_id })
      end

      # Bulk-cancel runs.
      # POST /v1/runs/bulk-cancel
      def bulk_cancel(body)
        _request(:post, "/v1/runs/bulk-cancel", body: body)
      end

      # Bulk-cancel all runs.
      # POST /v1/runs/bulk-cancel-all
      def bulk_cancel_all(body)
        _request(:post, "/v1/runs/bulk-cancel-all", body: body)
      end

      # Bulk DLQ replay.
      # POST /v1/runs/bulk-dlq-replay
      def bulk_dlq_replay(body)
        _request(:post, "/v1/runs/bulk-dlq-replay", body: body)
      end

      # Bulk-replay runs.
      # POST /v1/runs/bulk-replay
      def bulk_replay(body)
        _request(:post, "/v1/runs/bulk-replay", body: body)
      end

      # Get dead-letter queue.
      # GET /v1/runs/dlq
      def get_dlq(query: nil)
        _request(:get, "/v1/runs/dlq", query: query)
      end
    end
  end
end
