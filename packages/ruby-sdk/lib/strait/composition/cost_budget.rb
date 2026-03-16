# frozen_string_literal: true

module Strait
  module Composition
    # Options for cost budget tracking.
    CostBudgetOptions = Struct.new(:max_cost_microusd, :on_warning, :warning_threshold, keyword_init: true) do
      def initialize(**)
        super
        self.warning_threshold ||= 0.8
      end
    end

    # Tracks accumulated costs against a budget.
    class CostTracker
      def initialize(options)
        @current_cost = 0
        @options = options
        @warning_fired = false
      end

      def add(cost_microusd)
        @current_cost += cost_microusd

        threshold = (@options.max_cost_microusd * @options.warning_threshold).to_i
        if !@warning_fired && @options.on_warning && @current_cost >= threshold
          @warning_fired = true
          @options.on_warning.call(@current_cost, @options.max_cost_microusd)
        end

        if @current_cost >= @options.max_cost_microusd
          raise Strait::CostBudgetExceededError.new(
            "Cost budget exceeded: #{@current_cost} >= #{@options.max_cost_microusd} microusd",
            current_cost_microusd: @current_cost,
            max_cost_microusd: @options.max_cost_microusd
          )
        end
      end

      def current
        @current_cost
      end

      def remaining
        [@options.max_cost_microusd - @current_cost, 0].max
      end

      def exceeded?
        @current_cost >= @options.max_cost_microusd
      end
    end

    def self.create_cost_tracker(options)
      CostTracker.new(options)
    end

    def self.with_cost_budget(options, &block)
      tracker = create_cost_tracker(options)
      block.call(tracker)
    end
  end
end
