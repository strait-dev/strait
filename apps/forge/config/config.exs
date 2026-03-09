import Config

config :forge,
  grpc_port: 50051,
  max_concurrent_sandboxes: 50

import_config "#{config_env()}.exs"
