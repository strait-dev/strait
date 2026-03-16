# frozen_string_literal: true

module Strait
  module Operations
    # Service for workflow run lifecycle management and step-level operations.
    class WorkflowRunsService < BaseService
      # List all workflow runs.
      # GET /v1/workflow-runs
      def list(query: nil)
        _request(:get, "/v1/workflow-runs", query: query)
      end

      # Get a workflow run by ID.
      # GET /v1/workflow-runs/{workflowRunID}
      def get(workflow_run_id)
        _request(:get, "/v1/workflow-runs/{workflowRunID}", path_params: { "workflowRunID" => workflow_run_id })
      end

      # Delete a workflow run.
      # DELETE /v1/workflow-runs/{workflowRunID}
      def delete(workflow_run_id)
        _request(:delete, "/v1/workflow-runs/{workflowRunID}", path_params: { "workflowRunID" => workflow_run_id })
        nil
      end

      # Pause a workflow run.
      # POST /v1/workflow-runs/{workflowRunID}/pause
      def pause(workflow_run_id)
        _request(:post, "/v1/workflow-runs/{workflowRunID}/pause", path_params: { "workflowRunID" => workflow_run_id })
      end

      # Resume a paused workflow run.
      # POST /v1/workflow-runs/{workflowRunID}/resume
      def resume(workflow_run_id)
        _request(:post, "/v1/workflow-runs/{workflowRunID}/resume", path_params: { "workflowRunID" => workflow_run_id })
      end

      # Retry a failed workflow run.
      # POST /v1/workflow-runs/{workflowRunID}/retry
      def retry(workflow_run_id)
        _request(:post, "/v1/workflow-runs/{workflowRunID}/retry", path_params: { "workflowRunID" => workflow_run_id })
      end

      # List steps in a workflow run.
      # GET /v1/workflow-runs/{workflowRunID}/steps
      def list_steps(workflow_run_id)
        _request(:get, "/v1/workflow-runs/{workflowRunID}/steps", path_params: { "workflowRunID" => workflow_run_id })
      end

      # Approve a workflow step.
      # POST /v1/workflow-runs/{workflowRunID}/steps/{stepRef}/approve
      def approve_step(workflow_run_id, step_ref, body)
        _request(:post, "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/approve", path_params: { "workflowRunID" => workflow_run_id, "stepRef" => step_ref }, body: body)
      end

      # Retry a workflow step.
      # POST /v1/workflow-runs/{workflowRunID}/steps/{stepRef}/retry
      def retry_step(workflow_run_id, step_ref)
        _request(:post, "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/retry", path_params: { "workflowRunID" => workflow_run_id, "stepRef" => step_ref })
      end

      # Skip a workflow step.
      # POST /v1/workflow-runs/{workflowRunID}/steps/{stepRef}/skip
      def skip_step(workflow_run_id, step_ref)
        _request(:post, "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/skip", path_params: { "workflowRunID" => workflow_run_id, "stepRef" => step_ref })
      end

      # Force-complete a workflow step.
      # POST /v1/workflow-runs/{workflowRunID}/steps/{stepRef}/force-complete
      def force_complete_step(workflow_run_id, step_ref, body)
        _request(:post, "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/force-complete", path_params: { "workflowRunID" => workflow_run_id, "stepRef" => step_ref }, body: body)
      end

      # Replay the subtree starting from a workflow step.
      # POST /v1/workflow-runs/{workflowRunID}/steps/{stepRef}/replay-subtree
      def replay_subtree_step(workflow_run_id, step_ref)
        _request(:post, "/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/replay-subtree", path_params: { "workflowRunID" => workflow_run_id, "stepRef" => step_ref })
      end

      # Bulk-cancel workflow runs.
      # POST /v1/workflow-runs/bulk-cancel
      def bulk_cancel(body)
        _request(:post, "/v1/workflow-runs/bulk-cancel", body: body)
      end

      # Bulk-replay workflow runs.
      # POST /v1/workflow-runs/bulk-replay
      def bulk_replay(body)
        _request(:post, "/v1/workflow-runs/bulk-replay", body: body)
      end
    end
  end
end
