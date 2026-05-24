DROP POLICY IF EXISTS tenant_isolation ON tag_policies;
ALTER TABLE tag_policies NO FORCE ROW LEVEL SECURITY;
ALTER TABLE tag_policies DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON resource_policies;
ALTER TABLE resource_policies NO FORCE ROW LEVEL SECURITY;
ALTER TABLE resource_policies DISABLE ROW LEVEL SECURITY;

DROP INDEX IF EXISTS idx_resource_policies_project_resource;

ALTER TABLE resource_policies
    DROP CONSTRAINT IF EXISTS resource_policies_project_resource_user_key;

ALTER TABLE resource_policies
    ADD CONSTRAINT resource_policies_resource_type_resource_id_user_id_key
    UNIQUE (resource_type, resource_id, user_id);
