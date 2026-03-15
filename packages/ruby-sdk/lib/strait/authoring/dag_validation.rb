# frozen_string_literal: true

module Strait
  module Authoring
    def self.validate_dag(steps)
      refs = steps.map(&:ref)

      # Check duplicate refs
      seen = {}
      duplicates = []
      refs.each do |ref|
        if seen[ref]
          duplicates << ref
        end
        seen[ref] = true
      end
      unless duplicates.empty?
        raise Strait::DagValidationError.new(
          "duplicate step refs: #{duplicates.join(', ')}",
          duplicate_refs: duplicates
        )
      end

      all_refs = refs.to_set

      # Check missing refs
      missing = []
      steps.each do |step|
        step.depends_on.each do |dep|
          missing << dep unless all_refs.include?(dep)
        end
      end
      unless missing.empty?
        raise Strait::DagValidationError.new(
          "missing step refs: #{missing.join(', ')}",
          missing_refs: missing
        )
      end

      # Topological sort (Kahn's algorithm) to detect cycles
      in_degree = Hash.new(0)
      refs.each { |r| in_degree[r] = 0 }
      steps.each do |step|
        step.depends_on.each do |dep|
          in_degree[step.ref] += 1
        end
      end

      queue = refs.select { |r| in_degree[r] == 0 }
      sorted = []

      # Build adjacency list
      adj = Hash.new { |h, k| h[k] = [] }
      steps.each do |step|
        step.depends_on.each do |dep|
          adj[dep] << step.ref
        end
      end

      until queue.empty?
        node = queue.shift
        sorted << node
        adj[node].each do |neighbor|
          in_degree[neighbor] -= 1
          queue << neighbor if in_degree[neighbor] == 0
        end
      end

      if sorted.length != refs.length
        cycles = refs - sorted
        raise Strait::DagValidationError.new(
          "DAG contains cycles involving: #{cycles.join(', ')}",
          cycles: cycles
        )
      end

      sorted
    end
  end
end
