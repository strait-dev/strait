# frozen_string_literal: true

require "spec_helper"
require "strait/operations/analytics"
require "strait/operations/api_keys"
require "strait/operations/batch_operations"
require "strait/operations/deployments"
require "strait/operations/environments"
require "strait/operations/event_sources"
require "strait/operations/event_triggers"
require "strait/operations/health"
require "strait/operations/job_groups"
require "strait/operations/jobs"
require "strait/operations/log_drains"
require "strait/operations/rbac"
require "strait/operations/runs"
require "strait/operations/sdk_runs"
require "strait/operations/secrets"
require "strait/operations/stats"
require "strait/operations/webhooks"
require "strait/operations/workflow_runs"
require "strait/operations/workflows"

class MockClient
  attr_reader :last_method, :last_path, :last_body, :last_query

  def do_request(method, path, query: nil, headers: nil, body: nil)
    @last_method = method
    @last_path = path
    @last_query = query
    @last_body = body
    {}
  end
end

RSpec.describe Strait::Operations::AnalyticsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "get_performance sends GET /v1/analytics/performance" do
    service.get_performance
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/analytics/performance")
  end

  it "get_performance passes query params" do
    service.get_performance(query: { "from" => "2024-01-01" })
    expect(client.last_query).to eq({ "from" => "2024-01-01" })
  end
end

RSpec.describe Strait::Operations::APIKeysService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/api-keys" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/api-keys")
  end

  it "create sends POST /v1/api-keys" do
    service.create({ "name" => "my-key" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/api-keys")
    expect(client.last_body).to eq({ "name" => "my-key" })
  end

  it "delete sends DELETE /v1/api-keys/{keyID}" do
    service.delete("key_123")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/api-keys/key_123")
  end

  it "rotate sends POST /v1/api-keys/{keyID}/rotate" do
    service.rotate("key_123", { "ttl" => 3600 })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/api-keys/key_123/rotate")
    expect(client.last_body).to eq({ "ttl" => 3600 })
  end
end

RSpec.describe Strait::Operations::BatchOperationsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/batch-operations" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/batch-operations")
  end

  it "get sends GET /v1/batch-operations/{batchID}" do
    service.get("batch_123")
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/batch-operations/batch_123")
  end
end

RSpec.describe Strait::Operations::DeploymentsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/deployments" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/deployments")
  end

  it "create sends POST /v1/deployments" do
    service.create({ "project_id" => "proj_1" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/deployments")
    expect(client.last_body).to eq({ "project_id" => "proj_1" })
  end

  it "finalize sends POST /v1/deployments/{deploymentID}/finalize" do
    service.finalize("dep_123", { "status" => "ready" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/deployments/dep_123/finalize")
  end

  it "promote sends POST /v1/deployments/{deploymentID}/promote" do
    service.promote("dep_123", {})
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/deployments/dep_123/promote")
  end

  it "rollback sends POST /v1/deployments/{deploymentID}/rollback" do
    service.rollback("dep_123", {})
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/deployments/dep_123/rollback")
  end
end

RSpec.describe Strait::Operations::EnvironmentsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/environments" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/environments")
  end

  it "create sends POST /v1/environments" do
    service.create({ "name" => "staging" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/environments")
  end

  it "get sends GET /v1/environments/{envID}" do
    service.get("env_123")
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/environments/env_123")
  end

  it "update sends PATCH /v1/environments/{envID}" do
    service.update("env_123", { "name" => "prod" })
    expect(client.last_method).to eq(:patch)
    expect(client.last_path).to eq("/v1/environments/env_123")
  end

  it "delete sends DELETE /v1/environments/{envID}" do
    service.delete("env_123")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/environments/env_123")
  end
end

RSpec.describe Strait::Operations::EventSourcesService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/event-sources" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/event-sources")
  end

  it "create sends POST /v1/event-sources" do
    service.create({ "name" => "src" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/event-sources")
  end

  it "get sends GET /v1/event-sources/{sourceID}" do
    service.get("src_123")
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/event-sources/src_123")
  end

  it "delete sends DELETE /v1/event-sources/{sourceID}" do
    service.delete("src_123")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/event-sources/src_123")
  end
end

RSpec.describe Strait::Operations::EventTriggersService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list_events sends GET /v1/event-triggers/events" do
    service.list_events
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/event-triggers/events")
  end

  it "get_event sends GET /v1/event-triggers/events/{eventKey}" do
    service.get_event("payment.completed")
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/event-triggers/events/payment.completed")
  end

  it "delete_event sends DELETE /v1/event-triggers/events/{eventKey}" do
    service.delete_event("payment.completed")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/event-triggers/events/payment.completed")
  end

  it "send_event sends POST /v1/event-triggers/events/{eventKey}/send" do
    service.send_event("payment.completed", { "data" => "x" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/event-triggers/events/payment.completed/send")
  end
end

RSpec.describe Strait::Operations::HealthService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/health" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/health")
  end

  it "get_ready sends GET /v1/health/ready" do
    service.get_ready
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/health/ready")
  end

  it "list_metrics sends GET /v1/health/metrics" do
    service.list_metrics
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/health/metrics")
  end
end

RSpec.describe Strait::Operations::JobGroupsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/job-groups" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/job-groups")
  end

  it "create sends POST /v1/job-groups" do
    service.create({ "name" => "group1" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/job-groups")
  end

  it "get sends GET /v1/job-groups/{groupID}" do
    service.get("grp_123")
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/job-groups/grp_123")
  end

  it "delete sends DELETE /v1/job-groups/{groupID}" do
    service.delete("grp_123")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/job-groups/grp_123")
  end
end

RSpec.describe Strait::Operations::JobsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/jobs" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/jobs")
  end

  it "create sends POST /v1/jobs" do
    service.create({ "name" => "my-job", "slug" => "my-job" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/jobs")
  end

  it "get sends GET /v1/jobs/{jobID}" do
    service.get("job_123")
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/jobs/job_123")
  end

  it "update sends PATCH /v1/jobs/{jobID}" do
    service.update("job_123", { "name" => "updated" })
    expect(client.last_method).to eq(:patch)
    expect(client.last_path).to eq("/v1/jobs/job_123")
  end

  it "delete sends DELETE /v1/jobs/{jobID}" do
    service.delete("job_123")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/jobs/job_123")
  end

  it "trigger sends POST /v1/jobs/{jobID}/trigger" do
    service.trigger("job_123", { "payload" => {} })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/jobs/job_123/trigger")
  end

  it "bulk_trigger sends POST /v1/jobs/{jobID}/trigger/bulk" do
    service.bulk_trigger("job_123", { "items" => [] })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/jobs/job_123/trigger/bulk")
  end
end

RSpec.describe Strait::Operations::LogDrainsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/log-drains" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/log-drains")
  end

  it "create sends POST /v1/log-drains" do
    service.create({ "url" => "https://logs.example.com" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/log-drains")
  end

  it "get sends GET /v1/log-drains/{drainID}" do
    service.get("drain_123")
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/log-drains/drain_123")
  end

  it "delete sends DELETE /v1/log-drains/{drainID}" do
    service.delete("drain_123")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/log-drains/drain_123")
  end
end

RSpec.describe Strait::Operations::RBACService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list_audit_events sends GET /v1/rbac/audit-events" do
    service.list_audit_events
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/rbac/audit-events")
  end

  it "list_members sends GET /v1/rbac/members" do
    service.list_members
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/rbac/members")
  end

  it "create_member sends POST /v1/rbac/members" do
    service.create_member({ "email" => "user@example.com" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/rbac/members")
  end

  it "delete_member sends DELETE /v1/rbac/members/{userID}" do
    service.delete_member("user_123")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/rbac/members/user_123")
  end

  it "list_roles sends GET /v1/rbac/roles" do
    service.list_roles
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/rbac/roles")
  end
end

RSpec.describe Strait::Operations::RunsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/runs" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/runs")
  end

  it "get sends GET /v1/runs/{runID}" do
    service.get("run_123")
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/runs/run_123")
  end

  it "delete sends DELETE /v1/runs/{runID}" do
    service.delete("run_123")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/runs/run_123")
  end

  it "list_checkpoints sends GET /v1/runs/{runID}/checkpoints" do
    service.list_checkpoints("run_123")
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/runs/run_123/checkpoints")
  end

  it "replay sends POST /v1/runs/{runID}/replay" do
    service.replay("run_123", {})
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/runs/run_123/replay")
  end
end

RSpec.describe Strait::Operations::SDKRunsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "complete_run sends POST /sdk/v1/runs/{runID}/complete" do
    service.complete_run("run_123", { "output" => {} })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/sdk/v1/runs/run_123/complete")
  end

  it "fail_run sends POST /sdk/v1/runs/{runID}/fail" do
    service.fail_run("run_123", { "error" => "boom" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/sdk/v1/runs/run_123/fail")
  end

  it "heartbeat_run sends POST /sdk/v1/runs/{runID}/heartbeat" do
    service.heartbeat_run("run_123")
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/sdk/v1/runs/run_123/heartbeat")
  end

  it "annotate_run sends POST /sdk/v1/runs/{runID}/annotate" do
    service.annotate_run("run_123", { "key" => "value" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/sdk/v1/runs/run_123/annotate")
  end
end

RSpec.describe Strait::Operations::SecretsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/secrets" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/secrets")
  end

  it "create sends POST /v1/secrets" do
    service.create({ "name" => "DB_PASSWORD", "value" => "secret" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/secrets")
  end

  it "delete sends DELETE /v1/secrets/{secretID}" do
    service.delete("sec_123")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/secrets/sec_123")
  end
end

RSpec.describe Strait::Operations::StatsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/stats" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/stats")
  end

  it "list passes query params" do
    service.list(query: { "period" => "7d" })
    expect(client.last_query).to eq({ "period" => "7d" })
  end
end

RSpec.describe Strait::Operations::WebhooksService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list_subscriptions sends GET /v1/webhooks/subscriptions" do
    service.list_subscriptions
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/webhooks/subscriptions")
  end

  it "create_subscription sends POST /v1/webhooks/subscriptions" do
    service.create_subscription({ "url" => "https://hooks.example.com" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/webhooks/subscriptions")
  end

  it "delete_subscription sends DELETE /v1/webhooks/subscriptions/{id}" do
    service.delete_subscription("wh_123")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/webhooks/subscriptions/wh_123")
  end

  it "list_deliveries sends GET /v1/webhooks/deliveries" do
    service.list_deliveries
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/webhooks/deliveries")
  end
end

RSpec.describe Strait::Operations::WorkflowRunsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/workflow-runs" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/workflow-runs")
  end

  it "get sends GET /v1/workflow-runs/{workflowRunID}" do
    service.get("wfr_123")
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/workflow-runs/wfr_123")
  end

  it "delete sends DELETE /v1/workflow-runs/{workflowRunID}" do
    service.delete("wfr_123")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/workflow-runs/wfr_123")
  end

  it "pause sends POST /v1/workflow-runs/{workflowRunID}/pause" do
    service.pause("wfr_123")
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/workflow-runs/wfr_123/pause")
  end

  it "resume sends POST /v1/workflow-runs/{workflowRunID}/resume" do
    service.resume("wfr_123")
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/workflow-runs/wfr_123/resume")
  end
end

RSpec.describe Strait::Operations::WorkflowsService do
  let(:client) { MockClient.new }
  let(:service) { described_class.new(client) }

  it "list sends GET /v1/workflows" do
    service.list
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/workflows")
  end

  it "create sends POST /v1/workflows" do
    service.create({ "name" => "wf1", "slug" => "wf1" })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/workflows")
  end

  it "get sends GET /v1/workflows/{workflowID}" do
    service.get("wf_123")
    expect(client.last_method).to eq(:get)
    expect(client.last_path).to eq("/v1/workflows/wf_123")
  end

  it "update sends PATCH /v1/workflows/{workflowID}" do
    service.update("wf_123", { "name" => "updated" })
    expect(client.last_method).to eq(:patch)
    expect(client.last_path).to eq("/v1/workflows/wf_123")
  end

  it "delete sends DELETE /v1/workflows/{workflowID}" do
    service.delete("wf_123")
    expect(client.last_method).to eq(:delete)
    expect(client.last_path).to eq("/v1/workflows/wf_123")
  end

  it "trigger sends POST /v1/workflows/{workflowID}/trigger" do
    service.trigger("wf_123", { "payload" => {} })
    expect(client.last_method).to eq(:post)
    expect(client.last_path).to eq("/v1/workflows/wf_123/trigger")
  end
end
