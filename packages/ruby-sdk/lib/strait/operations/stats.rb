# frozen_string_literal: true

module Strait
  module Operations
    # Service for platform statistics.
    class StatsService < BaseService
      # List platform statistics.
      # GET /v1/stats
      def list(query: nil)
        _request(:get, "/v1/stats", query: query)
      end
    end
  end
end
