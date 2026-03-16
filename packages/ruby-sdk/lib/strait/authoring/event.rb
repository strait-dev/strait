# frozen_string_literal: true

module Strait
  module Authoring
    # An event definition with key and optional validator.
    class EventDefinition
      attr_reader :key

      def initialize(key, validate: nil)
        @key = key
        @validate = validate
      end

      def parse(input)
        @validate ? @validate.call(input) : input
      end
    end

    # Creates a new event definition.
    def self.define_event(key, validate: nil)
      EventDefinition.new(key, validate: validate)
    end
  end
end
