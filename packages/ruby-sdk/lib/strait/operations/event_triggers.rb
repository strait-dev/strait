# frozen_string_literal: true

module Strait
  module Operations
    # Service for event trigger management and dispatching.
    class EventTriggersService < BaseService
      # List all event trigger events.
      # GET /v1/event-triggers/events
      def list_events(query: nil)
        _request(:get, "/v1/event-triggers/events", query: query)
      end

      # Get an event by key.
      # GET /v1/event-triggers/events/{eventKey}
      def get_event(event_key)
        _request(:get, "/v1/event-triggers/events/{eventKey}", path_params: { "eventKey" => event_key })
      end

      # Delete an event by key.
      # DELETE /v1/event-triggers/events/{eventKey}
      def delete_event(event_key)
        _request(:delete, "/v1/event-triggers/events/{eventKey}", path_params: { "eventKey" => event_key })
        nil
      end

      # Send an event by key.
      # POST /v1/event-triggers/events/{eventKey}/send
      def send_event(event_key, body)
        _request(:post, "/v1/event-triggers/events/{eventKey}/send", path_params: { "eventKey" => event_key }, body: body)
      end

      # Send events matching a prefix.
      # POST /v1/event-triggers/prefix/{prefix}/send
      def send_prefix(prefix, body)
        _request(:post, "/v1/event-triggers/prefix/{prefix}/send", path_params: { "prefix" => prefix }, body: body)
      end

      # Purge event triggers.
      # POST /v1/event-triggers/purge
      def purge_event(body)
        _request(:post, "/v1/event-triggers/purge", body: body)
      end

      # Get event trigger statistics.
      # GET /v1/event-triggers/stats
      def get_stat
        _request(:get, "/v1/event-triggers/stats")
      end
    end
  end
end
