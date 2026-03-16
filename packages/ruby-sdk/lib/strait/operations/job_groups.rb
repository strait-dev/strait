# frozen_string_literal: true

module Strait
  module Operations
    # Service for job group CRUD, pause/resume, and statistics.
    class JobGroupsService < BaseService
      # List all job groups.
      # GET /v1/job-groups
      def list(query: nil)
        _request(:get, "/v1/job-groups", query: query)
      end

      # Create a new job group.
      # POST /v1/job-groups
      def create(body)
        _request(:post, "/v1/job-groups", body: body)
      end

      # Get a job group by ID.
      # GET /v1/job-groups/{groupID}
      def get(group_id)
        _request(:get, "/v1/job-groups/{groupID}", path_params: { "groupID" => group_id })
      end

      # Update a job group.
      # PATCH /v1/job-groups/{groupID}
      def update(group_id, body)
        _request(:patch, "/v1/job-groups/{groupID}", path_params: { "groupID" => group_id }, body: body)
      end

      # Delete a job group.
      # DELETE /v1/job-groups/{groupID}
      def delete(group_id)
        _request(:delete, "/v1/job-groups/{groupID}", path_params: { "groupID" => group_id })
        nil
      end

      # List jobs in a group.
      # GET /v1/job-groups/{groupID}/jobs
      def list_jobs(group_id, query: nil)
        _request(:get, "/v1/job-groups/{groupID}/jobs", path_params: { "groupID" => group_id }, query: query)
      end

      # Pause all jobs in a group.
      # POST /v1/job-groups/{groupID}/pause
      def pause_all(group_id)
        _request(:post, "/v1/job-groups/{groupID}/pause", path_params: { "groupID" => group_id })
      end

      # Resume all jobs in a group.
      # POST /v1/job-groups/{groupID}/resume
      def resume_all(group_id)
        _request(:post, "/v1/job-groups/{groupID}/resume", path_params: { "groupID" => group_id })
      end

      # Get statistics for a job group.
      # GET /v1/job-groups/{groupID}/stats
      def get_stats(group_id)
        _request(:get, "/v1/job-groups/{groupID}/stats", path_params: { "groupID" => group_id })
      end
    end
  end
end
