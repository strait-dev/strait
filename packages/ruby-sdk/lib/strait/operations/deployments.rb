# frozen_string_literal: true

module Strait
  module Operations
    # Service for deployment lifecycle management.
    class DeploymentsService < BaseService
      # List all deployments.
      # GET /v1/deployments
      def list(query: nil)
        _request(:get, "/v1/deployments", query: query)
      end

      # Create a new deployment.
      # POST /v1/deployments
      def create(body)
        _request(:post, "/v1/deployments", body: body)
      end

      # Finalize a deployment.
      # POST /v1/deployments/{deploymentID}/finalize
      def finalize(deployment_id, body)
        _request(:post, "/v1/deployments/{deploymentID}/finalize", path_params: { "deploymentID" => deployment_id }, body: body)
      end

      # Promote a deployment.
      # POST /v1/deployments/{deploymentID}/promote
      def promote(deployment_id, body)
        _request(:post, "/v1/deployments/{deploymentID}/promote", path_params: { "deploymentID" => deployment_id }, body: body)
      end

      # Rollback a deployment.
      # POST /v1/deployments/{deploymentID}/rollback
      def rollback(deployment_id, body)
        _request(:post, "/v1/deployments/{deploymentID}/rollback", path_params: { "deploymentID" => deployment_id }, body: body)
      end
    end
  end
end
