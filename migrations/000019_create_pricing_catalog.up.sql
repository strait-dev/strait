CREATE TABLE pricing_catalog (
    id                      TEXT        PRIMARY KEY,
    provider                TEXT        NOT NULL,
    model                   TEXT        NOT NULL,
    input_cost_microusd     BIGINT      NOT NULL,
    output_cost_microusd    BIGINT      NOT NULL,
    active                  BOOLEAN     NOT NULL DEFAULT TRUE,
    effective_from          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, model, effective_from)
);

CREATE INDEX idx_pricing_catalog_lookup ON pricing_catalog(provider, model, active, effective_from DESC);
