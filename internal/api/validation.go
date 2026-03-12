package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	maxJobNameLength  = 255
	maxJobSlugLength  = 128
	maxPayloadSize    = 5 * 1024 * 1024
	maxEndpointURLLen = 2048
)

func validateJobName(name string) error {
	if len(name) > maxJobNameLength {
		return fmt.Errorf("name too long (max %d characters)", maxJobNameLength)
	}
	return nil
}

func validateJobSlug(slug string) error {
	if len(slug) > maxJobSlugLength {
		return fmt.Errorf("slug too long (max %d characters)", maxJobSlugLength)
	}
	return nil
}

func validateEndpointNotEmpty(endpointURL string) error {
	if strings.TrimSpace(endpointURL) == "" {
		return fmt.Errorf("endpoint_url is required")
	}
	if len(endpointURL) > maxEndpointURLLen {
		return fmt.Errorf("endpoint_url too long (max %d characters)", maxEndpointURLLen)
	}
	return nil
}

func validatePayloadSize(payload json.RawMessage) error {
	if len(payload) > maxPayloadSize {
		return fmt.Errorf("payload too large (max %d bytes)", maxPayloadSize)
	}
	return nil
}

func validateRunCreationJobID(jobID string) error {
	if strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("job_id is required")
	}
	return nil
}
