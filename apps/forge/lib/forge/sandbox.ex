defmodule Forge.Sandbox do
  @moduledoc """
  Public API for sandbox code execution.

  Starts a supervised sandbox process and streams execution events
  back through the provided gRPC stream.
  """

  alias Forge.Sandbox.Supervisor, as: SandboxSupervisor

  @doc """
  Executes code in a sandboxed process.

  Returns `{:ok, stream}` when execution completes (events are streamed
  during execution), or `{:error, reason}` if the sandbox couldn't start.
  """
  @spec execute(map(), GRPC.Server.Stream.t()) ::
          {:ok, GRPC.Server.Stream.t()} | {:error, term()}
  def execute(opts, stream) do
    opts = Map.put(opts, :stream, stream)

    case SandboxSupervisor.start_execution(opts) do
      {:ok, pid} ->
        ref = Process.monitor(pid)

        receive do
          {:DOWN, ^ref, :process, ^pid, :normal} ->
            {:ok, stream}

          {:DOWN, ^ref, :process, ^pid, reason} ->
            {:error, {:sandbox_crashed, reason}}
        end

      {:error, reason} ->
        {:error, reason}
    end
  end
end
