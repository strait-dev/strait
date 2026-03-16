# frozen_string_literal: true

require "spec_helper"

RSpec.describe "Strait::Authoring.validate_dag" do
  def job_step(ref, deps = [])
    Strait::Authoring.job_step(ref, "job_#{ref}", depends_on: deps)
  end

  describe "valid DAGs" do
    it "validates a single step with no deps" do
      steps = [job_step("a")]
      result = Strait::Authoring.validate_dag(steps)
      expect(result).to eq(["a"])
    end

    it "validates a linear chain A -> B -> C" do
      steps = [
        job_step("a"),
        job_step("b", ["a"]),
        job_step("c", ["b"])
      ]
      result = Strait::Authoring.validate_dag(steps)
      expect(result).to eq(["a", "b", "c"])
    end

    it "validates a diamond DAG" do
      steps = [
        job_step("a"),
        job_step("b", ["a"]),
        job_step("c", ["a"]),
        job_step("d", ["b", "c"])
      ]
      result = Strait::Authoring.validate_dag(steps)
      expect(result.first).to eq("a")
      expect(result.last).to eq("d")
      expect(result.length).to eq(4)
    end

    it "validates parallel steps with no deps" do
      steps = [
        job_step("a"),
        job_step("b"),
        job_step("c")
      ]
      result = Strait::Authoring.validate_dag(steps)
      expect(result.length).to eq(3)
      expect(result).to contain_exactly("a", "b", "c")
    end

    it "validates a complex valid DAG" do
      steps = [
        job_step("a"),
        job_step("b", ["a"]),
        job_step("c", ["a"]),
        job_step("d", ["b"]),
        job_step("e", ["c", "d"]),
        job_step("f", ["e"])
      ]
      result = Strait::Authoring.validate_dag(steps)
      expect(result.length).to eq(6)
      expect(result.index("a")).to be < result.index("b")
      expect(result.index("a")).to be < result.index("c")
      expect(result.index("b")).to be < result.index("d")
      expect(result.index("d")).to be < result.index("e")
      expect(result.index("c")).to be < result.index("e")
      expect(result.index("e")).to be < result.index("f")
    end
  end

  describe "cycle detection" do
    it "detects a direct cycle A -> B -> A" do
      steps = [
        job_step("a", ["b"]),
        job_step("b", ["a"])
      ]
      expect {
        Strait::Authoring.validate_dag(steps)
      }.to raise_error(Strait::DagValidationError) { |e|
        expect(e.cycles).to include("a", "b")
      }
    end

    it "detects a three-node cycle A -> B -> C -> A" do
      steps = [
        job_step("a", ["c"]),
        job_step("b", ["a"]),
        job_step("c", ["b"])
      ]
      expect {
        Strait::Authoring.validate_dag(steps)
      }.to raise_error(Strait::DagValidationError) { |e|
        expect(e.cycles).to include("a", "b", "c")
      }
    end

    it "detects self-referencing step" do
      steps = [
        job_step("a", ["a"])
      ]
      expect {
        Strait::Authoring.validate_dag(steps)
      }.to raise_error(Strait::DagValidationError) { |e|
        expect(e.cycles).to include("a")
      }
    end
  end

  describe "duplicate ref detection" do
    it "detects duplicate step refs" do
      steps = [
        job_step("a"),
        job_step("a")
      ]
      expect {
        Strait::Authoring.validate_dag(steps)
      }.to raise_error(Strait::DagValidationError) { |e|
        expect(e.duplicate_refs).to include("a")
      }
    end

    it "detects multiple duplicates" do
      steps = [
        job_step("a"),
        job_step("b"),
        job_step("a"),
        job_step("b")
      ]
      expect {
        Strait::Authoring.validate_dag(steps)
      }.to raise_error(Strait::DagValidationError) { |e|
        expect(e.duplicate_refs).to include("a", "b")
      }
    end
  end

  describe "missing ref detection" do
    it "detects missing dependency ref" do
      steps = [
        job_step("a", ["nonexistent"])
      ]
      expect {
        Strait::Authoring.validate_dag(steps)
      }.to raise_error(Strait::DagValidationError) { |e|
        expect(e.missing_refs).to include("nonexistent")
      }
    end

    it "detects multiple missing refs" do
      steps = [
        job_step("a", ["x", "y"])
      ]
      expect {
        Strait::Authoring.validate_dag(steps)
      }.to raise_error(Strait::DagValidationError) { |e|
        expect(e.missing_refs).to include("x", "y")
      }
    end
  end

  describe "empty steps" do
    it "validates empty steps list" do
      result = Strait::Authoring.validate_dag([])
      expect(result).to eq([])
    end
  end
end
