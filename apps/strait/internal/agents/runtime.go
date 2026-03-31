package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const runtimeContractVersion = "v1"

type RuntimeDispatchEnvelope struct {
	Version    string                    `json:"version"`
	Run        RuntimeDispatchRun        `json:"run"`
	Agent      RuntimeDispatchAgent      `json:"agent"`
	Deployment RuntimeDispatchDeployment `json:"deployment"`
	Payload    json.RawMessage           `json:"payload,omitempty"`
	Callback   RuntimeDispatchCallback   `json:"callback"`
	Retry      *RuntimeDispatchRetry     `json:"retry,omitempty"`
}

type RuntimeDispatchRun struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Attempt     int    `json:"attempt"`
	TimeoutSecs int    `json:"timeout_secs"`
}

type RuntimeDispatchAgent struct {
	ID             string            `json:"id"`
	Slug           string            `json:"slug"`
	Model          string            `json:"model"`
	ModelFallbacks []string          `json:"model_fallbacks,omitempty"`
	Config         json.RawMessage   `json:"config,omitempty"`
	ProviderKeys   map[string]string `json:"provider_keys,omitempty"`
}

type RuntimeDispatchDeployment struct {
	ID             string          `json:"id"`
	Version        int             `json:"version"`
	Provider       string          `json:"provider"`
	ConfigSnapshot json.RawMessage `json:"config_snapshot,omitempty"`
	SandboxPolicy  json.RawMessage `json:"sandbox_policy,omitempty"`
}

type RuntimeDispatchCallback struct {
	BaseURL  string `json:"base_url"`
	RunID    string `json:"run_id"`
	RunToken string `json:"run_token"`
}

type RuntimeDispatchRetry struct {
	LastCheckpoint json.RawMessage `json:"last_checkpoint,omitempty"`
	CheckpointAt   string          `json:"checkpoint_at,omitempty"`
	PreviousError  string          `json:"previous_error,omitempty"`
}

type RuntimeEventType string

const (
	RuntimeEventCheckpoint RuntimeEventType = "checkpoint"
	RuntimeEventUsage      RuntimeEventType = "usage"
	RuntimeEventToolCall   RuntimeEventType = "tool_call"
	RuntimeEventStream     RuntimeEventType = "stream"
	RuntimeEventComplete   RuntimeEventType = "complete"
	RuntimeEventFail       RuntimeEventType = "fail"
)

type RuntimeEvent struct {
	Type RuntimeEventType `json:"type"`

	State json.RawMessage `json:"state,omitempty"`

	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	TotalTokens      int    `json:"total_tokens,omitempty"`
	CostMicrousd     int64  `json:"cost_microusd,omitempty"`

	ToolName   string          `json:"tool_name,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	DurationMs int             `json:"duration_ms,omitempty"`
	Status     string          `json:"status,omitempty"`

	Chunk    string `json:"chunk,omitempty"`
	StreamID string `json:"stream_id,omitempty"`
	Done     bool   `json:"done,omitempty"`

	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type runtimeEventState struct {
	terminal       bool
	completedIDs   map[string]struct{}
	terminalEvent  RuntimeEventType
	terminalResult bool
}

func (s *runtimeEventState) Validate(event *RuntimeEvent) error {
	if event == nil {
		return errors.New("runtime event is required")
	}
	if event.Type == "" {
		return errors.New("runtime event type is required")
	}
	if s.terminal {
		return fmt.Errorf("runtime event %q arrived after terminal event %q", event.Type, s.terminalEvent)
	}
	if s.completedIDs == nil {
		s.completedIDs = make(map[string]struct{})
	}

	switch event.Type {
	case RuntimeEventCheckpoint:
		if len(event.State) == 0 {
			return errors.New("checkpoint event requires state")
		}
	case RuntimeEventUsage:
		if strings.TrimSpace(event.Provider) == "" {
			return errors.New("usage event requires provider")
		}
		if strings.TrimSpace(event.Model) == "" {
			return errors.New("usage event requires model")
		}
	case RuntimeEventToolCall:
		if strings.TrimSpace(event.ToolName) == "" {
			return errors.New("tool_call event requires tool_name")
		}
		if strings.TrimSpace(event.Status) == "" {
			event.Status = "completed"
		}
	case RuntimeEventStream:
		if strings.TrimSpace(event.StreamID) == "" {
			event.StreamID = "default"
		}
		if _, exists := s.completedIDs[event.StreamID]; exists {
			return fmt.Errorf("stream %q received chunks after done", event.StreamID)
		}
		if event.Chunk == "" && !event.Done {
			return errors.New("stream event requires chunk or done=true")
		}
		if event.Done {
			s.completedIDs[event.StreamID] = struct{}{}
		}
	case RuntimeEventComplete:
		s.terminal = true
		s.terminalEvent = event.Type
		s.terminalResult = true
	case RuntimeEventFail:
		if strings.TrimSpace(event.Error) == "" {
			return errors.New("fail event requires error")
		}
		s.terminal = true
		s.terminalEvent = event.Type
		s.terminalResult = true
	default:
		return fmt.Errorf("unknown runtime event type %q", event.Type)
	}

	return nil
}
