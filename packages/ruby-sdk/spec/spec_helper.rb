# frozen_string_literal: true

require "set"
require "strait/version"
require "strait/errors"
require "strait/config"
require "strait/config_file"
require "strait/http"
require "strait/middleware"
require "strait/operations/base"
require "strait/fsm/run"
require "strait/fsm/workflow"
require "strait/fsm/step"
require "strait/authoring/run_context"
require "strait/authoring/run_context_client"
require "strait/authoring/steps"
require "strait/authoring/dag_validation"
require "strait/authoring/job"
require "strait/authoring/workflow"
require "strait/authoring/agent"
require "strait/authoring/event"
require "strait/authoring/test_helpers"
require "strait/composition/result"
require "strait/composition/retry"
require "strait/composition/paginate"
require "strait/composition/idempotency"
require "strait/composition/deployments"
require "strait/composition/cost_budget"
require "strait/composition/checkpoint_resume"

RSpec.configure do |config|
  config.expect_with :rspec do |expectations|
    expectations.include_chain_clauses_in_custom_matcher_descriptions = true
  end
end
