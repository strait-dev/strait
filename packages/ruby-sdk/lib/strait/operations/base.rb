# frozen_string_literal: true

module Strait
  module Operations
    # Base class for all operation services. Provides shared request helpers.
    class BaseService
      # @param client [Strait::Client] the API client instance
      def initialize(client)
        @client = client
      end

      private

      # Build and execute an API request.
      #
      # @param method      [Symbol]     HTTP method (:get, :post, :patch, :put, :delete)
      # @param path        [String]     URL path, may contain {param} placeholders
      # @param path_params [Hash, nil]  values to substitute into path placeholders
      # @param query       [Hash, nil]  query-string parameters
      # @param headers     [Hash, nil]  additional request headers
      # @param body        [Hash, nil]  request body (will be JSON-encoded)
      # @return [Hash, nil] parsed JSON response
      def _request(method, path, path_params: nil, query: nil, headers: nil, body: nil)
        resolved_path = if path_params
                          Strait::HTTP.substitute_path_params(path, path_params)
                        else
                          path
                        end

        @client.do_request(method, resolved_path, query: query, headers: headers, body: body)
      end
    end
  end
end
