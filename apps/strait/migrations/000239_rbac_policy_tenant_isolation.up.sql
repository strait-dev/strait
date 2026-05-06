ALTER TABLE resource_policies
    DROP CONSTRAINT IF EXISTS resource_policies_resource_type_resource_id_user_id_key;

ALTER TABLE resource_policies
    ADD CONSTRAINT resource_policies_project_resource_user_key
    UNIQUE (project_id, resource_type, resource_id, user_id);

CREATE INDEX IF NOT EXISTS idx_resource_policies_project_resource
    ON resource_policies(project_id, resource_type, resource_id);

ALTER TABLE resource_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE resource_policies FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON resource_policies;
CREATE POLICY tenant_isolation ON resource_policies
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');

ALTER TABLE tag_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE tag_policies FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON tag_policies;
CREATE POLICY tenant_isolation ON tag_policies
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');
