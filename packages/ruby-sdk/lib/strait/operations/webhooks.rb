# frozen_string_literal: true

module Strait
  module Operations
    # Service for webhook subscription and delivery management.
    class WebhooksService < BaseService
      # List webhook subscriptions.
      # GET /v1/webhooks/subscriptions
      def list_subscriptions(query: nil)
        _request(:get, "/v1/webhooks/subscriptions", query: query)
      end

      # Create a webhook subscription.
      # POST /v1/webhooks/subscriptions
      def create_subscription(body)
        _request(:post, "/v1/webhooks/subscriptions", body: body)
      end

      # Delete a webhook subscription.
      # DELETE /v1/webhooks/subscriptions/{id}
      def delete_subscription(id)
        _request(:delete, "/v1/webhooks/subscriptions/{id}", path_params: { "id" => id })
        nil
      end

      # List webhook deliveries.
      # GET /v1/webhooks/deliveries
      def list_deliveries(query: nil)
        _request(:get, "/v1/webhooks/deliveries", query: query)
      end

      # Get a webhook delivery by ID.
      # GET /v1/webhooks/deliveries/{id}
      def get_delivery(id)
        _request(:get, "/v1/webhooks/deliveries/{id}", path_params: { "id" => id })
      end

      # Retry a webhook delivery.
      # POST /v1/webhooks/deliveries/{id}/retry
      def retry_delivery(id)
        _request(:post, "/v1/webhooks/deliveries/{id}/retry", path_params: { "id" => id })
      end
    end
  end
end
