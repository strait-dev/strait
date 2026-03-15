# frozen_string_literal: true

module Strait
  module Authoring
    RunContext = Struct.new(:run_id, :attempt, keyword_init: true)
  end
end
