# frozen_string_literal: true

module Strait
  module Operations
    # Service for job CRUD, triggering, versioning, and dependency management.
    class JobsService < BaseService
      # List all jobs.
      # GET /v1/jobs
      def list(query: nil)
        _request(:get, "/v1/jobs", query: query)
      end

      # Create a new job.
      # POST /v1/jobs
      def create(body)
        _request(:post, "/v1/jobs", body: body)
      end

      # Get a job by ID.
      # GET /v1/jobs/{jobID}
      def get(job_id)
        _request(:get, "/v1/jobs/{jobID}", path_params: { "jobID" => job_id })
      end

      # Update a job.
      # PATCH /v1/jobs/{jobID}
      def update(job_id, body)
        _request(:patch, "/v1/jobs/{jobID}", path_params: { "jobID" => job_id }, body: body)
      end

      # Delete a job.
      # DELETE /v1/jobs/{jobID}
      def delete(job_id)
        _request(:delete, "/v1/jobs/{jobID}", path_params: { "jobID" => job_id })
        nil
      end

      # Clone a job.
      # POST /v1/jobs/{jobID}/clone
      def clone(job_id, body)
        _request(:post, "/v1/jobs/{jobID}/clone", path_params: { "jobID" => job_id }, body: body)
      end

      # Get health status of a job.
      # GET /v1/jobs/{jobID}/health
      def get_health(job_id)
        _request(:get, "/v1/jobs/{jobID}/health", path_params: { "jobID" => job_id })
      end

      # Trigger a job run.
      # POST /v1/jobs/{jobID}/trigger
      def trigger(job_id, body)
        _request(:post, "/v1/jobs/{jobID}/trigger", path_params: { "jobID" => job_id }, body: body)
      end

      # Bulk-trigger a job.
      # POST /v1/jobs/{jobID}/trigger/bulk
      def bulk_trigger(job_id, body)
        _request(:post, "/v1/jobs/{jobID}/trigger/bulk", path_params: { "jobID" => job_id }, body: body)
      end

      # List versions of a job.
      # GET /v1/jobs/{jobID}/versions
      def list_versions(job_id, query: nil)
        _request(:get, "/v1/jobs/{jobID}/versions", path_params: { "jobID" => job_id }, query: query)
      end

      # Get a specific version of a job.
      # GET /v1/jobs/{jobID}/versions/{versionID}
      def get_version(job_id, version_id)
        _request(:get, "/v1/jobs/{jobID}/versions/{versionID}", path_params: { "jobID" => job_id, "versionID" => version_id })
      end

      # List dependencies for a job.
      # GET /v1/jobs/{jobID}/dependencies
      def list_dependencies(job_id)
        _request(:get, "/v1/jobs/{jobID}/dependencies", path_params: { "jobID" => job_id })
      end

      # Create a dependency for a job.
      # POST /v1/jobs/{jobID}/dependencies
      def create_dependency(job_id, body)
        _request(:post, "/v1/jobs/{jobID}/dependencies", path_params: { "jobID" => job_id }, body: body)
      end

      # Delete a dependency from a job.
      # DELETE /v1/jobs/{jobID}/dependencies/{depID}
      def delete_dependency(job_id, dep_id)
        _request(:delete, "/v1/jobs/{jobID}/dependencies/{depID}", path_params: { "jobID" => job_id, "depID" => dep_id })
        nil
      end

      # Batch-create jobs.
      # POST /v1/jobs/batch
      def batch(body)
        _request(:post, "/v1/jobs/batch", body: body)
      end

      # Batch-disable jobs.
      # POST /v1/jobs/batch-disable
      def batch_disable(body)
        _request(:post, "/v1/jobs/batch-disable", body: body)
      end

      # Batch-enable jobs.
      # POST /v1/jobs/batch-enable
      def batch_enable(body)
        _request(:post, "/v1/jobs/batch-enable", body: body)
      end
    end
  end
end
