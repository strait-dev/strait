# frozen_string_literal: true

module Strait
  module Operations
    # Service for event source CRUD, subscriptions, and dispatching.
    class EventSourcesService < BaseService
      # List all event sources.
      # GET /v1/event-sources
      def list(query: nil)
        _request(:get, "/v1/event-sources", query: query)
      end

      # Create a new event source.
      # POST /v1/event-sources
      def create(body)
        _request(:post, "/v1/event-sources", body: body)
      end

      # Get an event source by ID.
      # GET /v1/event-sources/{sourceID}
      def get(source_id)
        _request(:get, "/v1/event-sources/{sourceID}", path_params: { "sourceID" => source_id })
      end

      # Update an event source.
      # PATCH /v1/event-sources/{sourceID}
      def update(source_id, body)
        _request(:patch, "/v1/event-sources/{sourceID}", path_params: { "sourceID" => source_id }, body: body)
      end

      # Delete an event source.
      # DELETE /v1/event-sources/{sourceID}
      def delete(source_id)
        _request(:delete, "/v1/event-sources/{sourceID}", path_params: { "sourceID" => source_id })
        nil
      end

      # Subscribe to an event source.
      # POST /v1/event-sources/{sourceID}/subscribe
      def subscribe(source_id, body)
        _request(:post, "/v1/event-sources/{sourceID}/subscribe", path_params: { "sourceID" => source_id }, body: body)
      end

      # List subscriptions for an event source.
      # GET /v1/event-sources/{sourceID}/subscriptions
      def list_subscriptions(source_id)
        _request(:get, "/v1/event-sources/{sourceID}/subscriptions", path_params: { "sourceID" => source_id })
      end

      # Delete a subscription from an event source.
      # DELETE /v1/event-sources/{sourceID}/subscriptions/{subID}
      def delete_subscription(source_id, sub_id)
        _request(:delete, "/v1/event-sources/{sourceID}/subscriptions/{subID}", path_params: { "sourceID" => source_id, "subID" => sub_id })
        nil
      end

      # Dispatch an event from a source.
      # POST /v1/event-sources/dispatch
      def dispatch_event(body)
        _request(:post, "/v1/event-sources/dispatch", body: body)
      end
    end
  end
end
