# frozen_string_literal: true

module Strait
  module Operations
    # Service for log drain CRUD.
    class LogDrainsService < BaseService
      # List all log drains.
      # GET /v1/log-drains
      def list(query: nil)
        _request(:get, "/v1/log-drains", query: query)
      end

      # Create a new log drain.
      # POST /v1/log-drains
      def create(body)
        _request(:post, "/v1/log-drains", body: body)
      end

      # Get a log drain by ID.
      # GET /v1/log-drains/{drainID}
      def get(drain_id)
        _request(:get, "/v1/log-drains/{drainID}", path_params: { "drainID" => drain_id })
      end

      # Update a log drain.
      # PATCH /v1/log-drains/{drainID}
      def update(drain_id, body)
        _request(:patch, "/v1/log-drains/{drainID}", path_params: { "drainID" => drain_id }, body: body)
      end

      # Delete a log drain.
      # DELETE /v1/log-drains/{drainID}
      def delete(drain_id)
        _request(:delete, "/v1/log-drains/{drainID}", path_params: { "drainID" => drain_id })
        nil
      end
    end
  end
end
