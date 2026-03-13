-- RBAC: project roles and member assignments.
CREATE TABLE project_roles (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT,
    permissions TEXT[] NOT NULL DEFAULT '{}',
    is_system   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, name)
);

CREATE INDEX idx_project_roles_project ON project_roles(project_id);

CREATE TABLE project_member_roles (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    user_id     TEXT NOT NULL,
    role_id     TEXT NOT NULL REFERENCES project_roles(id) ON DELETE CASCADE,
    granted_by  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, user_id)
);

CREATE INDEX idx_member_roles_user ON project_member_roles(user_id);
CREATE INDEX idx_member_roles_project ON project_member_roles(project_id);

-- Fine-grained per-resource policies.
CREATE TABLE resource_policies (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL,
    resource_type  TEXT NOT NULL,
    resource_id    TEXT NOT NULL,
    user_id        TEXT NOT NULL,
    actions        TEXT[] NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (resource_type, resource_id, user_id)
);

CREATE INDEX idx_resource_policies_resource ON resource_policies(resource_type, resource_id);
CREATE INDEX idx_resource_policies_user ON resource_policies(user_id);
