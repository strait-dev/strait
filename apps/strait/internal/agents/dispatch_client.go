package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"strait/internal/domain"
)

func (s *localService) dispatchCloudflareRun(ctx context.Context, deployment *domain.AgentDeployment, envelope RuntimeDispatchEnvelope) error {
	if s.dispatchHTTP == nil {
		return fmt.Errorf("cloudflare dispatch http client is not configured")
	}
	if strings.TrimSpace(s.internalSecret) == "" {
		return fmt.Errorf("internal secret is required for cloudflare dispatch")
	}

	metadata, err := ParseCloudflareDeploymentMetadata(deployment.ProviderMetadata)
	if err != nil {
		return fmt.Errorf("parse cloudflare deployment metadata: %w", err)
	}

	payload, err := json.Marshal(CloudflareDispatchRequest{
		DeploymentID:  deployment.ID,
		Provider:      deployment.Provider,
		Namespace:     metadata.Namespace,
		ScriptName:    metadata.ScriptName,
		RunID:         envelope.Run.ID,
		SandboxPolicy: metadata.SandboxPolicy,
		Envelope:      envelope,
	})
	if err != nil {
		return fmt.Errorf("marshal cloudflare dispatch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, metadata.DispatchWorkerURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build cloudflare dispatch request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.internalSecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.dispatchHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("dispatch cloudflare runtime request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("cloudflare dispatch worker returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
