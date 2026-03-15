# frozen_string_literal: true

module Strait
  module Operations
    # Service for analytics and performance metrics.
    class AnalyticsService < BaseService
      # Get performance analytics.
      # GET /v1/analytics/performance
      def get_performance(query: nil)
        _request(:get, "/v1/analytics/performance", query: query)
      end
    end
  end
end
