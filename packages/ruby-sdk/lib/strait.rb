# frozen_string_literal: true

require "set"

require_relative "strait/version"
require_relative "strait/config"
require_relative "strait/config_file"
require_relative "strait/errors"
require_relative "strait/http"
require_relative "strait/middleware"

# Operation services
require_relative "strait/operations/base"
require_relative "strait/operations/health"
require_relative "strait/operations/jobs"
require_relative "strait/operations/runs"
require_relative "strait/operations/workflows"
require_relative "strait/operations/workflow_runs"
require_relative "strait/operations/deployments"
require_relative "strait/operations/environments"
require_relative "strait/operations/secrets"
require_relative "strait/operations/api_keys"
require_relative "strait/operations/webhooks"
require_relative "strait/operations/event_triggers"
require_relative "strait/operations/event_sources"
require_relative "strait/operations/batch_operations"
require_relative "strait/operations/stats"
require_relative "strait/operations/analytics"
require_relative "strait/operations/log_drains"
require_relative "strait/operations/sdk_runs"
require_relative "strait/operations/rbac"
require_relative "strait/operations/job_groups"

# FSM state machines
require_relative "strait/fsm/run"
require_relative "strait/fsm/workflow"
require_relative "strait/fsm/step"

# Authoring DSL
require_relative "strait/authoring/run_context"
require_relative "strait/authoring/run_context_client"
require_relative "strait/authoring/steps"
require_relative "strait/authoring/dag_validation"
require_relative "strait/authoring/job"
require_relative "strait/authoring/workflow"
require_relative "strait/authoring/agent"
require_relative "strait/authoring/event"
require_relative "strait/authoring/test_helpers"

# Composition helpers
require_relative "strait/composition/result"
require_relative "strait/composition/retry"
require_relative "strait/composition/wait"
require_relative "strait/composition/trigger"
require_relative "strait/composition/paginate"
require_relative "strait/composition/idempotency"
require_relative "strait/composition/deployments"
require_relative "strait/composition/cost_budget"
require_relative "strait/composition/checkpoint_resume"

# Client (depends on all the above)
require_relative "strait/client"
