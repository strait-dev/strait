# frozen_string_literal: true

module Strait
  module Operations
    # Service for health-check endpoints.
    class HealthService < BaseService
      # List overall health status.
      # GET /v1/health
      def list
        _request(:get, "/v1/health")
      end

      # Check readiness probe.
      # GET /v1/health/ready
      def get_ready
        _request(:get, "/v1/health/ready")
      end

      # List runtime metrics.
      # GET /v1/health/metrics
      def list_metrics
        _request(:get, "/v1/health/metrics")
      end
    end
  end
end
