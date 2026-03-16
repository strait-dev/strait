# frozen_string_literal: true

module Strait
  module Operations
    # Service for API key management.
    class APIKeysService < BaseService
      # List all API keys.
      # GET /v1/api-keys
      def list(query: nil)
        _request(:get, "/v1/api-keys", query: query)
      end

      # Create a new API key.
      # POST /v1/api-keys
      def create(body)
        _request(:post, "/v1/api-keys", body: body)
      end

      # Delete an API key.
      # DELETE /v1/api-keys/{keyID}
      def delete(key_id)
        _request(:delete, "/v1/api-keys/{keyID}", path_params: { "keyID" => key_id })
        nil
      end

      # Rotate an API key.
      # POST /v1/api-keys/{keyID}/rotate
      def rotate(key_id, body)
        _request(:post, "/v1/api-keys/{keyID}/rotate", path_params: { "keyID" => key_id }, body: body)
      end
    end
  end
end
