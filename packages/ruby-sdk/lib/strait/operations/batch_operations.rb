# frozen_string_literal: true

module Strait
  module Operations
    # Service for batch operation tracking.
    class BatchOperationsService < BaseService
      # List all batch operations.
      # GET /v1/batch-operations
      def list(query: nil)
        _request(:get, "/v1/batch-operations", query: query)
      end

      # Get a batch operation by ID.
      # GET /v1/batch-operations/{batchID}
      def get(batch_id)
        _request(:get, "/v1/batch-operations/{batchID}", path_params: { "batchID" => batch_id })
      end
    end
  end
end
