import Config

config :forge,
  grpc_port: String.to_integer(System.get_env("GRPC_PORT") || "50051"),
  max_concurrent_sandboxes: String.to_integer(System.get_env("MAX_SANDBOXES") || "50")

config :logger, level: :info
