defmodule Forge.Application do
  @moduledoc """
  Forge application supervisor.

  Starts the gRPC server and sandbox supervision tree.
  """
  use Application

  require Logger

  @impl true
  def start(_type, _args) do
    port = Application.get_env(:forge, :grpc_port, 50051)

    children = [
      {Forge.Sandbox.Supervisor, []},
      {GRPC.Server.Supervisor,
       endpoint: Forge.GRPC.Endpoint,
       port: port,
       start_server: true}
    ]

    opts = [strategy: :one_for_one, name: Forge.Supervisor]
    Logger.info("Forge starting on port #{port}")
    Supervisor.start_link(children, opts)
  end
end
