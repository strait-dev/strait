package api

import "encoding/json"

func sanitizedJobUpdateAuditChanges(req UpdateJobRequest) map[string]any {
	secretChanged := req.EndpointSigningSecret != nil || req.WebhookSecret != nil
	req.EndpointSigningSecret = nil
	req.WebhookSecret = nil

	raw, err := json.Marshal(req)
	if err != nil {
		return map[string]any{"marshal_error": true, "signing_credential_changed": secretChanged}
	}
	var changes map[string]any
	if err := json.Unmarshal(raw, &changes); err != nil {
		return map[string]any{"marshal_error": true, "signing_credential_changed": secretChanged}
	}
	if changes == nil {
		changes = map[string]any{}
	}
	if secretChanged {
		changes["signing_credential_changed"] = true
	}
	return changes
}
