defmodule Forge.Application do
  @moduledoc """
  Forge application supervisor.

  Starts the gRPC server and sandbox supervision tree.
  """
  use Application

  require Logger

  @impl true
  def start(_type, _args) do
    children = [
      {Forge.Sandbox.Supervisor, []}
    ] ++ grpc_children()

    opts = [strategy: :one_for_one, name: Forge.Supervisor]
    Logger.info("Forge starting")
    Supervisor.start_link(children, opts)
  end

  defp grpc_children do
    if Application.get_env(:forge, :start_grpc, true) do
      port = Application.get_env(:forge, :grpc_port, 50051)

      [
        {GRPC.Server.Supervisor,
         endpoint: Forge.GRPC.Endpoint,
         port: port,
         start_server: true}
      ]
    else
      []
    end
  end
end
