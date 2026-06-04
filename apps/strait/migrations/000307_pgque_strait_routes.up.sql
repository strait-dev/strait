CREATE TABLE IF NOT EXISTS strait_pgque_routes (
    route_key   TEXT PRIMARY KEY,
    queue_name  TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE strait_pgque_routes IS
    'Maps Strait queue routes to PgQue physical queue names. PgQue stores ready events; Strait owns run state and execution ownership.';

