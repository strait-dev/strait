defmodule Forge.GRPC.SandboxServer do
  @moduledoc """
  gRPC server implementation for the SandboxExecutor service.

  Handles Execute RPCs by spawning supervised sandbox processes
  and streaming execution events back to the caller.
  """
  use GRPC.Server, service: Sandbox.V1.SandboxExecutor.Service

  require Logger

  alias Forge.Sandbox

  @spec execute(Sandbox.V1.ExecuteRequest.t(), GRPC.Server.Stream.t()) ::
          {:ok, GRPC.Server.Stream.t()} | {:error, GRPC.RPCError.t()}
  def execute(request, stream) do
    Logger.info("Executing sandbox run=#{request.run_id} lang=#{request.language}")

    opts = %{
      run_id: request.run_id,
      language: request.language,
      code: request.code,
      payload: request.payload,
      env: request.env,
      timeout_ms: (request.limits && request.limits.timeout_secs * 1_000) || 30_000,
      memory_bytes: (request.limits && request.limits.memory_bytes) || 256 * 1024 * 1024,
      network_enabled: (request.limits && request.limits.network_enabled) || false
    }

    case Sandbox.execute(opts, stream) do
      {:ok, stream} ->
        {:ok, stream}

      {:error, :max_children} ->
        Logger.warning("Sandbox at capacity run=#{request.run_id}")
        raise GRPC.RPCError, status: :resource_exhausted, message: "sandbox at capacity"

      {:error, reason} ->
        Logger.error("Sandbox execution failed run=#{request.run_id}: #{inspect(reason)}")
        raise GRPC.RPCError, status: :internal, message: "execution failed: #{inspect(reason)}"
    end
  end
end
