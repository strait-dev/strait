CREATE TABLE workflow_run_labels (
    workflow_run_id TEXT NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    label_key       TEXT NOT NULL,
    label_value     TEXT NOT NULL,
    PRIMARY KEY (workflow_run_id, label_key)
);

CREATE INDEX idx_workflow_run_labels_key_value ON workflow_run_labels(label_key, label_value);
