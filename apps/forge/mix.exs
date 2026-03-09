defmodule Forge.MixProject do
  use Mix.Project

  def project do
    [
      app: :forge,
      version: "0.1.0",
      elixir: "~> 1.17 or ~> 1.19",
      start_permanent: Mix.env() == :prod,
      deps: deps(),
      aliases: aliases(),
      elixirc_paths: elixirc_paths(Mix.env()),
      test_coverage: [
        ignore_modules: [
          Sandbox.V1.Checkpoint,
          Sandbox.V1.ExecuteRequest,
          Sandbox.V1.ExecuteRequest.EnvEntry,
          Sandbox.V1.ExecutionEvent,
          Sandbox.V1.ExecutionResult,
          Sandbox.V1.LogEntry,
          Sandbox.V1.ResourceLimits,
          Sandbox.V1.SandboxExecutor.Service,
          Sandbox.V1.SandboxExecutor.Stub,
          Sandbox.V1.ToolCall,
          Forge.GRPC.Endpoint,
          Forge.GRPC.SandboxServer
        ],
        threshold: 90.0
      ]
    ]
  end

  def application do
    [
      extra_applications: [:logger],
      mod: {Forge.Application, []}
    ]
  end

  defp elixirc_paths(:test), do: ["lib", "test/support"]
  defp elixirc_paths(_), do: ["lib"]

  defp deps do
    [
      {:grpc, "~> 0.9"},
      {:protobuf, "~> 0.13"},
      {:jason, "~> 1.4"},
      {:telemetry, "~> 1.3"},
      {:credo, "~> 1.7", only: [:dev, :test], runtime: false}
    ]
  end

  defp aliases do
    []
  end
end
