CREATE TABLE IF NOT EXISTS strait_pgque_ready_events (
    run_id           TEXT NOT NULL,
    ready_generation BIGINT NOT NULL,
    emitted_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, ready_generation)
);

COMMENT ON TABLE strait_pgque_ready_events IS
    'Records pgque ready emits by run generation so the claim reconciler can repair stranded runs without repeatedly duplicating backlog events.';
