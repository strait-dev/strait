# frozen_string_literal: true

module Strait
  module Composition
    CreateAndFinalizeOutput = Struct.new(:created, :finalized, keyword_init: true)
    CreateFinalizePromoteOutput = Struct.new(:created, :finalized, :promoted, keyword_init: true)

    def self.create_and_finalize_deployment(client, create_body, finalize_body: nil)
      finalize_body ||= infer_mutation_body(create_body)
      created = client.deployments.create(create_body)
      deployment_id = created["id"]
      finalized = client.deployments.finalize(deployment_id, finalize_body)
      CreateAndFinalizeOutput.new(created: created, finalized: finalized)
    end

    def self.create_finalize_promote_deployment(client, create_body, finalize_body: nil, promote_body: nil)
      finalize_body ||= infer_mutation_body(create_body)
      promote_body ||= infer_mutation_body(create_body)
      created = client.deployments.create(create_body)
      deployment_id = created["id"]
      finalized = client.deployments.finalize(deployment_id, finalize_body)
      promoted = client.deployments.promote(deployment_id, promote_body)
      CreateFinalizePromoteOutput.new(created: created, finalized: finalized, promoted: promoted)
    end

    private_class_method def self.infer_mutation_body(create_body)
      body = {}
      body["project_id"] = create_body["project_id"] if create_body["project_id"]
      body["environment"] = create_body["environment"] if create_body["environment"]
      body
    end
  end
end
