package agents

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	maxAgentNameLength = 255
	maxAgentSlugLength = 128
	maxAgentModelLen   = 255
	maxAgentConfigSize = 1 << 20
)

type ValidationError struct {
	field   string
	message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s %s", e.field, e.message)
}

func validateCreateRequest(req CreateAgentRequest) error {
	if strings.TrimSpace(req.ProjectID) == "" {
		return &ValidationError{field: "project_id", message: "is required"}
	}
	if err := validateName(req.Name); err != nil {
		return err
	}
	if err := validateSlug(req.Slug); err != nil {
		return err
	}
	if err := validateModel(req.Model); err != nil {
		return err
	}
	if err := validateModelFallbacks(req.ModelFallbacks); err != nil {
		return err
	}
	if err := validateProviderSecrets(req.ProviderSecrets); err != nil {
		return err
	}
	if err := validateCron(req.Cron); err != nil {
		return err
	}
	if err := validateCronTimezone(req.CronTimezone); err != nil {
		return err
	}
	return validateConfig(req.Config)
}

func validateUpdateRequest(req UpdateAgentRequest) error {
	if strings.TrimSpace(req.AgentID) == "" {
		return &ValidationError{field: "agent_id", message: "is required"}
	}
	return validateCreateRequest(CreateAgentRequest{
		ProjectID:       req.ProjectID,
		Name:            req.Name,
		Slug:            req.Slug,
		Model:           req.Model,
		ModelFallbacks:  req.ModelFallbacks,
		Config:          req.Config,
		ProviderSecrets: req.ProviderSecrets,
		Cron:            req.Cron,
		CronTimezone:    req.CronTimezone,
	})
}

func validateRunRequest(req RunAgentRequest) error {
	if strings.TrimSpace(req.ProjectID) == "" {
		return &ValidationError{field: "project_id", message: "is required"}
	}
	if strings.TrimSpace(req.AgentID) == "" {
		return &ValidationError{field: "agent_id", message: "is required"}
	}
	if len(req.Payload) == 0 {
		return nil
	}
	if len(req.Payload) > maxAgentConfigSize {
		return &ValidationError{field: "payload", message: fmt.Sprintf("too large (max %d bytes)", maxAgentConfigSize)}
	}
	if !json.Valid(req.Payload) {
		return &ValidationError{field: "payload", message: "must be valid JSON"}
	}
	return nil
}

func validateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return &ValidationError{field: "name", message: "is required"}
	}
	if len(name) > maxAgentNameLength {
		return &ValidationError{field: "name", message: fmt.Sprintf("too long (max %d characters)", maxAgentNameLength)}
	}
	return nil
}

func validateSlug(slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return &ValidationError{field: "slug", message: "is required"}
	}
	if len(slug) > maxAgentSlugLength {
		return &ValidationError{field: "slug", message: fmt.Sprintf("too long (max %d characters)", maxAgentSlugLength)}
	}
	return nil
}

func validateModel(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return &ValidationError{field: "model", message: "is required"}
	}
	if len(model) > maxAgentModelLen {
		return &ValidationError{field: "model", message: fmt.Sprintf("too long (max %d characters)", maxAgentModelLen)}
	}
	return nil
}

func validateConfig(config json.RawMessage) error {
	if len(config) == 0 {
		return nil
	}
	if len(config) > maxAgentConfigSize {
		return &ValidationError{field: "config", message: fmt.Sprintf("too large (max %d bytes)", maxAgentConfigSize)}
	}
	if !json.Valid(config) {
		return &ValidationError{field: "config", message: "must be valid JSON"}
	}
	var decoded any
	if err := json.Unmarshal(config, &decoded); err != nil {
		return &ValidationError{field: "config", message: "must be valid JSON"}
	}
	if _, ok := decoded.(map[string]any); !ok {
		return &ValidationError{field: "config", message: "must be a JSON object"}
	}
	return nil
}

const (
	maxModelFallbacks  = 5
	maxProviderSecrets = 10
)

func validateModelFallbacks(fallbacks []string) error {
	if len(fallbacks) > maxModelFallbacks {
		return &ValidationError{field: "model_fallbacks", message: fmt.Sprintf("too many fallbacks (max %d)", maxModelFallbacks)}
	}
	for i, model := range fallbacks {
		if err := validateModel(model); err != nil {
			return &ValidationError{field: "model_fallbacks", message: fmt.Sprintf("fallback[%d]: %s", i, err.Error())}
		}
	}
	return nil
}

func validateProviderSecrets(secrets map[string]string) error {
	if len(secrets) > maxProviderSecrets {
		return &ValidationError{field: "provider_secrets", message: fmt.Sprintf("too many providers (max %d)", maxProviderSecrets)}
	}
	for k, v := range secrets {
		if strings.TrimSpace(k) == "" {
			return &ValidationError{field: "provider_secrets", message: "provider name must not be empty"}
		}
		if strings.TrimSpace(v) == "" {
			return &ValidationError{field: "provider_secrets", message: fmt.Sprintf("secret for provider %q must not be empty", k)}
		}
	}
	return nil
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func validateCron(expr string) error {
	if expr == "" {
		return nil
	}
	if _, err := cronParser.Parse(expr); err != nil {
		return &ValidationError{field: "cron", message: fmt.Sprintf("invalid cron expression: %v", err)}
	}
	return nil
}

func validateCronTimezone(tz string) error {
	if tz == "" {
		return nil
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return &ValidationError{field: "cron_timezone", message: fmt.Sprintf("invalid timezone: %v", err)}
	}
	return nil
}
