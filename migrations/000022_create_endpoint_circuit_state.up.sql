CREATE TABLE endpoint_circuit_state (
    endpoint_url            TEXT        PRIMARY KEY,
    state                   TEXT        NOT NULL DEFAULT 'closed',
    consecutive_failures    INT         NOT NULL DEFAULT 0,
    opened_at               TIMESTAMPTZ,
    half_open_until         TIMESTAMPTZ,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_endpoint_circuit_state_state ON endpoint_circuit_state(state);
