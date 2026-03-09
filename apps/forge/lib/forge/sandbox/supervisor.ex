defmodule Forge.Sandbox.Supervisor do
  @moduledoc """
  Dynamic supervisor for sandbox execution processes.

  Each code execution runs as a supervised child process with
  resource limits and automatic cleanup on termination.
  """
  use DynamicSupervisor

  def start_link(opts) do
    DynamicSupervisor.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @impl true
  def init(_opts) do
    DynamicSupervisor.init(
      strategy: :one_for_one,
      max_children: Application.get_env(:forge, :max_concurrent_sandboxes, 50)
    )
  end

  @doc """
  Starts a new sandbox execution process under supervision.
  """
  def start_execution(opts) do
    spec = {Forge.Sandbox.Runner, opts}
    DynamicSupervisor.start_child(__MODULE__, spec)
  end
end
