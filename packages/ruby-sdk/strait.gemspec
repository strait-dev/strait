require_relative "lib/strait/version"

Gem::Specification.new do |spec|
  spec.name = "strait"
  spec.version = Strait::VERSION
  spec.authors = ["Strait"]
  spec.email = ["support@strait.dev"]
  spec.summary = "Ruby SDK for the Strait API"
  spec.description = "Official Ruby SDK for the Strait job orchestration platform. Provides typed client, authoring DSL, composition helpers, FSM state machines, and 186 API operations across 19 services."
  spec.homepage = "https://github.com/strait-dev/ruby-sdk"
  spec.license = "MIT"
  spec.required_ruby_version = ">= 3.1"

  spec.files = Dir["lib/**/*.rb", "LICENSE", "README.md"]
  spec.require_paths = ["lib"]

  spec.add_dependency "faraday", "~> 2.0"
  spec.add_dependency "json", ">= 2.0"
end
