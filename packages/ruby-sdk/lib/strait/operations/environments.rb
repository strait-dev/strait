# frozen_string_literal: true

module Strait
  module Operations
    # Service for environment CRUD and variable listing.
    class EnvironmentsService < BaseService
      # List all environments.
      # GET /v1/environments
      def list(query: nil)
        _request(:get, "/v1/environments", query: query)
      end

      # Create a new environment.
      # POST /v1/environments
      def create(body)
        _request(:post, "/v1/environments", body: body)
      end

      # Get an environment by ID.
      # GET /v1/environments/{envID}
      def get(env_id)
        _request(:get, "/v1/environments/{envID}", path_params: { "envID" => env_id })
      end

      # Update an environment.
      # PATCH /v1/environments/{envID}
      def update(env_id, body)
        _request(:patch, "/v1/environments/{envID}", path_params: { "envID" => env_id }, body: body)
      end

      # Delete an environment.
      # DELETE /v1/environments/{envID}
      def delete(env_id)
        _request(:delete, "/v1/environments/{envID}", path_params: { "envID" => env_id })
        nil
      end

      # List variables for an environment.
      # GET /v1/environments/{envID}/variables
      def list_variables(env_id)
        _request(:get, "/v1/environments/{envID}/variables", path_params: { "envID" => env_id })
      end
    end
  end
end
