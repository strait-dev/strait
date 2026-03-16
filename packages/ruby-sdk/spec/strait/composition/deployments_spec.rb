# frozen_string_literal: true

require "spec_helper"

RSpec.describe "Strait::Composition deployments" do
  # Mock deployments service
  let(:deployments_service) do
    svc = Object.new
    def svc.create(body)
      { "id" => "dep_123", "status" => "created" }
    end
    def svc.finalize(id, body)
      { "id" => id, "status" => "finalized" }
    end
    def svc.promote(id, body)
      { "id" => id, "status" => "promoted" }
    end
    svc
  end

  let(:client) do
    c = Object.new
    ds = deployments_service
    c.define_singleton_method(:deployments) { ds }
    c
  end

  describe ".create_and_finalize_deployment" do
    it "creates and finalizes a deployment" do
      result = Strait::Composition.create_and_finalize_deployment(
        client,
        { "project_id" => "proj_1", "environment" => "production" }
      )
      expect(result).to be_a(Strait::Composition::CreateAndFinalizeOutput)
      expect(result.created["id"]).to eq("dep_123")
      expect(result.finalized["status"]).to eq("finalized")
    end
  end

  describe ".create_finalize_promote_deployment" do
    it "creates, finalizes, and promotes a deployment" do
      result = Strait::Composition.create_finalize_promote_deployment(
        client,
        { "project_id" => "proj_1", "environment" => "production" }
      )
      expect(result).to be_a(Strait::Composition::CreateFinalizePromoteOutput)
      expect(result.created["id"]).to eq("dep_123")
      expect(result.finalized["status"]).to eq("finalized")
      expect(result.promoted["status"]).to eq("promoted")
    end
  end

  describe "CreateAndFinalizeOutput" do
    it "has created and finalized fields" do
      output = Strait::Composition::CreateAndFinalizeOutput.new(created: "c", finalized: "f")
      expect(output.created).to eq("c")
      expect(output.finalized).to eq("f")
    end
  end

  describe "CreateFinalizePromoteOutput" do
    it "has created, finalized, and promoted fields" do
      output = Strait::Composition::CreateFinalizePromoteOutput.new(created: "c", finalized: "f", promoted: "p")
      expect(output.created).to eq("c")
      expect(output.finalized).to eq("f")
      expect(output.promoted).to eq("p")
    end
  end
end
