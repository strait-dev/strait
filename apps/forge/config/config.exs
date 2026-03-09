import Config

config :forge,
  grpc_port: 50051,
  max_concurrent_sandboxes: 50

config :grpc, start_server: true

import_config "#{config_env()}.exs"
