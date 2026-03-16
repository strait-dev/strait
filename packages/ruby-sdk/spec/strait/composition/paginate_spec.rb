# frozen_string_literal: true

require "spec_helper"

RSpec.describe "Strait::Composition pagination" do
  describe Strait::Composition::PaginatedQuery do
    it "creates a PaginatedQuery struct" do
      query = Strait::Composition::PaginatedQuery.new(cursor: "abc", limit: 25)
      expect(query.cursor).to eq("abc")
      expect(query.limit).to eq(25)
    end
  end

  describe Strait::Composition::PaginatedResponse do
    it "creates a PaginatedResponse struct" do
      resp = Strait::Composition::PaginatedResponse.new(
        data: [1, 2, 3],
        items: nil,
        next_cursor: "next_abc",
        has_more: true
      )
      expect(resp.data).to eq([1, 2, 3])
      expect(resp.next_cursor).to eq("next_abc")
      expect(resp.has_more).to be true
    end
  end

  describe Strait::Composition::PaginateOptions do
    it "defaults limit to 50" do
      opts = Strait::Composition::PaginateOptions.new
      expect(opts.limit).to eq(50)
    end

    it "accepts custom limit" do
      opts = Strait::Composition::PaginateOptions.new(limit: 100)
      expect(opts.limit).to eq(100)
    end
  end

  describe "Strait::Composition.paginate" do
    it "yields items from a single page" do
      items = Strait::Composition.paginate do |_query|
        Strait::Composition::PaginatedResponse.new(
          items: ["a", "b", "c"],
          has_more: false,
          next_cursor: nil
        )
      end
      expect(items.to_a).to eq(["a", "b", "c"])
    end

    it "handles multiple pages" do
      call_count = 0
      items = Strait::Composition.paginate do |query|
        call_count += 1
        if query.cursor.nil?
          Strait::Composition::PaginatedResponse.new(
            items: [1, 2],
            has_more: true,
            next_cursor: "page2"
          )
        else
          Strait::Composition::PaginatedResponse.new(
            items: [3, 4],
            has_more: false,
            next_cursor: nil
          )
        end
      end
      expect(items.to_a).to eq([1, 2, 3, 4])
      expect(call_count).to eq(2)
    end

    it "stops when has_more is false" do
      call_count = 0
      items = Strait::Composition.paginate do |_query|
        call_count += 1
        Strait::Composition::PaginatedResponse.new(
          items: ["x"],
          has_more: false,
          next_cursor: "should_not_follow"
        )
      end
      expect(items.to_a).to eq(["x"])
      expect(call_count).to eq(1)
    end

    it "uses data field when items is nil" do
      items = Strait::Composition.paginate do |_query|
        Strait::Composition::PaginatedResponse.new(
          data: ["d1", "d2"],
          items: nil,
          has_more: false,
          next_cursor: nil
        )
      end
      expect(items.to_a).to eq(["d1", "d2"])
    end
  end

  describe "Strait::Composition.collect_all" do
    it "collects all items from multiple pages" do
      call_count = 0
      result = Strait::Composition.collect_all do |query|
        call_count += 1
        if query.cursor.nil?
          Strait::Composition::PaginatedResponse.new(items: [1], has_more: true, next_cursor: "p2")
        elsif query.cursor == "p2"
          Strait::Composition::PaginatedResponse.new(items: [2], has_more: true, next_cursor: "p3")
        else
          Strait::Composition::PaginatedResponse.new(items: [3], has_more: false, next_cursor: nil)
        end
      end
      expect(result).to eq([1, 2, 3])
      expect(call_count).to eq(3)
    end
  end
end
