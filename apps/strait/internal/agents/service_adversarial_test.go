package agents

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateRunRequestRejectsOversizedPayload(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`"` + strings.Repeat("a", maxAgentConfigSize) + `"`)
	err := validateRunRequest(RunAgentRequest{
		ProjectID: "proj-1",
		AgentID:   "agent-1",
		Payload:   payload,
	})
	if err == nil {
		t.Fatal("expected oversized payload error")
	}
}

func TestValidateCreateRequestRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	err := validateCreateRequest(CreateAgentRequest{
		ProjectID: "proj-1",
		Name:      "Support Agent",
		Slug:      "support-agent",
		Model:     "gpt-5.4",
		Config:    json.RawMessage(`{"temperature":`),
	})
	if err == nil {
		t.Fatal("expected malformed JSON error")
	}
}

func TestValidateCreateRequestRejectsEmptySlug(t *testing.T) {
	t.Parallel()

	err := validateCreateRequest(CreateAgentRequest{
		ProjectID: "proj-1",
		Name:      "Support Agent",
		Slug:      "   ",
		Model:     "gpt-5.4",
	})
	if err == nil {
		t.Fatal("expected slug validation error")
	}
}
