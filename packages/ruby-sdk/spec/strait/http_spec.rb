# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::HTTP do
  describe ".substitute_path_params" do
    it "substitutes a single param" do
      result = Strait::HTTP.substitute_path_params("/v1/jobs/{jobID}", { "jobID" => "job_123" })
      expect(result).to eq("/v1/jobs/job_123")
    end

    it "substitutes multiple params" do
      result = Strait::HTTP.substitute_path_params(
        "/v1/jobs/{jobID}/versions/{versionID}",
        { "jobID" => "job_123", "versionID" => "v_456" }
      )
      expect(result).to eq("/v1/jobs/job_123/versions/v_456")
    end

    it "returns path unchanged with no params" do
      result = Strait::HTTP.substitute_path_params("/v1/health", {})
      expect(result).to eq("/v1/health")
    end

    it "does not modify path when param key is not found" do
      result = Strait::HTTP.substitute_path_params("/v1/jobs/{jobID}", { "other" => "val" })
      expect(result).to eq("/v1/jobs/{jobID}")
    end

    it "handles empty params hash" do
      result = Strait::HTTP.substitute_path_params("/v1/jobs/{jobID}", {})
      expect(result).to eq("/v1/jobs/{jobID}")
    end

    it "converts non-string values to string" do
      result = Strait::HTTP.substitute_path_params("/v1/items/{id}", { "id" => 42 })
      expect(result).to eq("/v1/items/42")
    end

    it "does not mutate the original path string" do
      original = "/v1/jobs/{jobID}"
      Strait::HTTP.substitute_path_params(original, { "jobID" => "job_123" })
      expect(original).to eq("/v1/jobs/{jobID}")
    end

    it "handles path with no placeholders" do
      result = Strait::HTTP.substitute_path_params("/v1/health", { "jobID" => "job_123" })
      expect(result).to eq("/v1/health")
    end
  end
end
