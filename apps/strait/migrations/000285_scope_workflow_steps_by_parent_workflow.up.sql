ALTER TABLE workflow_steps ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_steps FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON workflow_steps;
CREATE POLICY tenant_isolation ON workflow_steps
	USING (
		EXISTS (
			SELECT 1
			FROM workflows w
			WHERE w.id = workflow_id
			  AND (
			  	w.project_id = current_setting('app.current_project_id', true)
			  	OR current_setting('app.current_project_id', true) = ''
			  )
		)
	)
	WITH CHECK (
		EXISTS (
			SELECT 1
			FROM workflows w
			WHERE w.id = workflow_id
			  AND (
			  	w.project_id = current_setting('app.current_project_id', true)
			  	OR current_setting('app.current_project_id', true) = ''
			  )
		)
	);
