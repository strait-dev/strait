defmodule Forge.GRPC.Endpoint do
  @moduledoc """
  gRPC endpoint for the Forge sandbox service.
  """
  use GRPC.Endpoint

  intercept(GRPC.Server.Interceptors.Logger)

  run(Forge.GRPC.SandboxServer)
end
