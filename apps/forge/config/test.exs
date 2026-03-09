import Config

config :forge,
  grpc_port: 50052,
  max_concurrent_sandboxes: 5,
  start_grpc: false

config :logger, level: :warning
