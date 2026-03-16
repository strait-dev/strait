package authoring

// DefineDag creates a DAG-flavored workflow definition.
// It is a thin wrapper over DefineWorkflow with Kind set to "dag".
func DefineDag[TPayload any](opts WorkflowOptions[TPayload]) *WorkflowDefinition[TPayload] {
	wf := DefineWorkflow(opts)
	wf.Kind = "dag"
	return wf
}
