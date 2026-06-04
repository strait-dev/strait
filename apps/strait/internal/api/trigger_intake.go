package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

func mergedRunTags(base, overlay map[string]string) map[string]string {
	runTags := make(map[string]string, len(base)+len(overlay))
	maps.Copy(runTags, base)
	maps.Copy(runTags, overlay)
	return runTags
}

func mergeRunMetadata(metadata, defaults map[string]string) map[string]string {
	merged := make(map[string]string, len(defaults)+len(metadata))
	maps.Copy(merged, metadata)
	for key, value := range defaults {
		if _, exists := merged[key]; !exists {
			merged[key] = value
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func ensureJobTriggerable(job *domain.Job) error {
	if !job.Enabled {
		return huma.Error400BadRequest("job is disabled")
	}
	if job.Paused {
		return huma.Error409Conflict("job is paused -- resume it before triggering new runs")
	}
	return nil
}

func (s *Server) validateTriggerJobInput(input *TriggerJobInput, req *TriggerRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return newValidationError(err)
	}
	if err := validateTriggerTraceHeaders(input); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validatePayloadSize(req.Payload); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validateTags(req.Tags); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validateTriggerScheduledAt(req.ScheduledAt); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	return nil
}

func validateTriggerScheduledAt(scheduledAt *time.Time) error {
	if scheduledAt == nil {
		return nil
	}
	delay := time.Until(*scheduledAt)
	if delay < 0 {
		return errors.New("scheduled_at must not be in the past")
	}
	if delay > 30*24*time.Hour {
		return errors.New("scheduled_at cannot exceed 30 days from now")
	}
	return nil
}

func validateTriggerTTLSecs(ttlSecs *int) error {
	if ttlSecs == nil {
		return nil
	}
	if *ttlSecs < 0 {
		return errors.New("ttl_secs must be greater than or equal to 0")
	}
	if *ttlSecs > maxTriggerTTLSecs {
		return errors.New("ttl_secs cannot exceed 30 days")
	}
	return nil
}

func canonicalizePayload(payload json.RawMessage) (json.RawMessage, string, error) {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	var v any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&v); err != nil {
		return nil, "", err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, "", errors.New("payload must contain a single JSON value")
	}

	canonical, err := json.Marshal(v)
	if err != nil {
		return nil, "", err
	}

	hash := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(hash[:]), nil
}
