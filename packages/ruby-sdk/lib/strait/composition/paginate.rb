# frozen_string_literal: true

module Strait
  module Composition
    PaginatedQuery = Struct.new(:cursor, :limit, keyword_init: true)

    PaginatedResponse = Struct.new(:data, :items, :next_cursor, :has_more, keyword_init: true)

    PaginateOptions = Struct.new(:limit, keyword_init: true) do
      def initialize(**kwargs)
        super
        self.limit ||= 50
      end
    end

    def self.paginate(opts = nil, &list_fn)
      opts ||= PaginateOptions.new
      Enumerator.new do |yielder|
        cursor = nil
        loop do
          query = PaginatedQuery.new(cursor: cursor, limit: opts.limit)
          response = list_fn.call(query)
          items = response.items || response.data || []
          items.each { |item| yielder << item }

          break unless response.has_more
          break if response.next_cursor.nil? || response.next_cursor.empty?
          cursor = response.next_cursor
        end
      end
    end

    def self.collect_all(opts = nil, &list_fn)
      paginate(opts, &list_fn).to_a
    end
  end
end
