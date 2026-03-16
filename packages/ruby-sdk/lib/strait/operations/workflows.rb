# frozen_string_literal: true

module Strait
  module Operations
    # Service for workflow CRUD, triggering, versioning, simulation, and policy management.
    class WorkflowsService < BaseService
      # List all workflows.
      # GET /v1/workflows
      def list(query: nil)
        _request(:get, "/v1/workflows", query: query)
      end

      # Create a new workflow.
      # POST /v1/workflows
      def create(body)
        _request(:post, "/v1/workflows", body: body)
      end

      # Get a workflow by ID.
      # GET /v1/workflows/{workflowID}
      def get(workflow_id)
        _request(:get, "/v1/workflows/{workflowID}", path_params: { "workflowID" => workflow_id })
      end

      # Update a workflow.
      # PATCH /v1/workflows/{workflowID}
      def update(workflow_id, body)
        _request(:patch, "/v1/workflows/{workflowID}", path_params: { "workflowID" => workflow_id }, body: body)
      end

      # Delete a workflow.
      # DELETE /v1/workflows/{workflowID}
      def delete(workflow_id)
        _request(:delete, "/v1/workflows/{workflowID}", path_params: { "workflowID" => workflow_id })
        nil
      end

      # Clone a workflow.
      # POST /v1/workflows/{workflowID}/clone
      def clone(workflow_id, body)
        _request(:post, "/v1/workflows/{workflowID}/clone", path_params: { "workflowID" => workflow_id }, body: body)
      end

      # Perform a dry run of a workflow.
      # POST /v1/workflows/{workflowID}/dry-run
      def dry_run(workflow_id, body)
        _request(:post, "/v1/workflows/{workflowID}/dry-run", path_params: { "workflowID" => workflow_id }, body: body)
      end

      # Generate an execution plan for a workflow.
      # POST /v1/workflows/{workflowID}/plan
      def plan(workflow_id, body)
        _request(:post, "/v1/workflows/{workflowID}/plan", path_params: { "workflowID" => workflow_id }, body: body)
      end

      # Simulate a workflow execution.
      # POST /v1/workflows/{workflowID}/simulate
      def simulate(workflow_id, body)
        _request(:post, "/v1/workflows/{workflowID}/simulate", path_params: { "workflowID" => workflow_id }, body: body)
      end

      # Trigger a workflow run.
      # POST /v1/workflows/{workflowID}/trigger
      def trigger(workflow_id, body)
        _request(:post, "/v1/workflows/{workflowID}/trigger", path_params: { "workflowID" => workflow_id }, body: body)
      end

      # Get the graph for a workflow run.
      # GET /v1/workflow-runs/{workflowRunID}/graph
      def get_graph(workflow_run_id)
        _request(:get, "/v1/workflow-runs/{workflowRunID}/graph", path_params: { "workflowRunID" => workflow_run_id })
      end

      # Get the graph for a workflow by workflow ID.
      # GET /v1/workflows/{workflowID}/graph
      def get_graph_by_workflow_id(workflow_id)
        _request(:get, "/v1/workflows/{workflowID}/graph", path_params: { "workflowID" => workflow_id })
      end

      # List runs for a workflow.
      # GET /v1/workflows/{workflowID}/runs
      def list_runs(workflow_id, query: nil)
        _request(:get, "/v1/workflows/{workflowID}/runs", path_params: { "workflowID" => workflow_id }, query: query)
      end

      # List versions of a workflow.
      # GET /v1/workflows/{workflowID}/versions
      def list_versions(workflow_id, query: nil)
        _request(:get, "/v1/workflows/{workflowID}/versions", path_params: { "workflowID" => workflow_id }, query: query)
      end

      # Get a specific version of a workflow.
      # GET /v1/workflows/{workflowID}/versions/{versionID}
      def get_version(workflow_id, version_id)
        _request(:get, "/v1/workflows/{workflowID}/versions/{versionID}", path_params: { "workflowID" => workflow_id, "versionID" => version_id })
      end

      # Get the diff between two workflow versions.
      # GET /v1/workflows/{workflowID}/versions/{fromVersionID}/diff/{toVersionID}
      def get_diff(workflow_id, from_version_id, to_version_id)
        _request(:get, "/v1/workflows/{workflowID}/versions/{fromVersionID}/diff/{toVersionID}", path_params: {
          "workflowID" => workflow_id,
          "fromVersionID" => from_version_id,
          "toVersionID" => to_version_id
        })
      end

      # Get the workflow policy for a project.
      # GET /v1/projects/{projectID}/workflow-policy
      def get_policy(project_id)
        _request(:get, "/v1/projects/{projectID}/workflow-policy", path_params: { "projectID" => project_id })
      end

      # Create or update the workflow policy for a project.
      # PUT /v1/projects/{projectID}/workflow-policy
      def upsert_policy(project_id, body)
        _request(:put, "/v1/projects/{projectID}/workflow-policy", path_params: { "projectID" => project_id }, body: body)
      end

      # Get an explanation of a workflow run.
      # GET /v1/workflow-runs/{workflowRunID}/explain
      def get_explain(workflow_run_id)
        _request(:get, "/v1/workflow-runs/{workflowRunID}/explain", path_params: { "workflowRunID" => workflow_run_id })
      end

      # List labels for a workflow run.
      # GET /v1/workflow-runs/{workflowRunID}/labels
      def list_labels(workflow_run_id)
        _request(:get, "/v1/workflow-runs/{workflowRunID}/labels", path_params: { "workflowRunID" => workflow_run_id })
      end

      # Get impact analysis for a workflow version.
      # GET /v1/workflows/{workflowID}/versions/{versionID}/impact
      def get_impact(workflow_id, version_id)
        _request(:get, "/v1/workflows/{workflowID}/versions/{versionID}/impact", path_params: { "workflowID" => workflow_id, "versionID" => version_id })
      end

      # List steps for a workflow version.
      # GET /v1/workflows/{workflowID}/versions/{versionID}/steps
      def list_steps_by_version(workflow_id, version_id)
        _request(:get, "/v1/workflows/{workflowID}/versions/{versionID}/steps", path_params: { "workflowID" => workflow_id, "versionID" => version_id })
      end
    end
  end
end
