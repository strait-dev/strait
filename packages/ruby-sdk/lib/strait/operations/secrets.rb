# frozen_string_literal: true

module Strait
  module Operations
    # Service for secret management.
    class SecretsService < BaseService
      # List all secrets.
      # GET /v1/secrets
      def list(query: nil)
        _request(:get, "/v1/secrets", query: query)
      end

      # Create a new secret.
      # POST /v1/secrets
      def create(body)
        _request(:post, "/v1/secrets", body: body)
      end

      # Delete a secret.
      # DELETE /v1/secrets/{secretID}
      def delete(secret_id)
        _request(:delete, "/v1/secrets/{secretID}", path_params: { "secretID" => secret_id })
        nil
      end
    end
  end
end
