package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/store"
	"strait/internal/worker"

	"github.com/danielgtaylor/huma/v2"
)

// SetJobEndpointRequest is the request body for setting a job's HTTP endpoint.
type SetJobEndpointRequest struct {
	EndpointURL         string `json:"endpoint_url" validate:"required,url"`
	FallbackEndpointURL string `json:"fallback_endpoint_url,omitempty" validate:"omitempty,url"`
	// RotateSigningSecret, when true, generates a fresh HMAC signing secret for
	// the job and returns it once in the response. When false (default), the
	// existing signing secret is preserved. URL-only updates do not rotate.
	RotateSigningSecret bool `json:"rotate_signing_secret,omitempty"`
}

// SetJobEndpointInput is the typed input for the set-endpoint route.
type SetJobEndpointInput struct {
	JobID string `path:"jobID"`
	Body  SetJobEndpointRequest
}

// SetJobEndpointResponse is the response body for the set-endpoint route.
type SetJobEndpointResponse struct {
	Job *domain.Job `json:"job"`
	// SigningSecret is populated only when RotateSigningSecret was true on the
	// request. Stored encrypted server-side; returned once and never again.
	SigningSecret string `json:"signing_secret,omitempty"`
}

// SetJobEndpointOutput is the typed output for the set-endpoint route.
type SetJobEndpointOutput struct {
	Body *SetJobEndpointResponse
}

func (s *Server) encryptEndpointSigningSecret(secret string) (string, error) {
	if secret == "" {
		return secret, nil
	}
	if s.encryptor == nil {
		return "", fmt.Errorf("endpoint signing secret encryption is not configured")
	}
	return straitcrypto.EncryptField(s.encryptor, secret)
}

func (s *Server) preserveOrEncryptEndpointSigningSecret(secret string) (string, error) {
	if secret == "" || straitcrypto.IsEncryptedField(secret) {
		return secret, nil
	}
	return straitcrypto.PreserveOrEncryptField(s.encryptor, secret)
}

func (s *Server) decryptEndpointSigningSecret(secret string) (string, error) {
	return straitcrypto.DecryptField(s.encryptor, secret)
}

// VerifyJobEndpointInput is the typed input for the verify-endpoint route.
type VerifyJobEndpointInput struct {
	JobID string `path:"jobID"`
}

// VerifyJobEndpointResult is the body returned by the verify-endpoint route.
type VerifyJobEndpointResult struct {
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code,omitempty"`
	LatencyMs  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
}

// VerifyJobEndpointOutput is the typed output for the verify-endpoint route.
type VerifyJobEndpointOutput struct {
	Body VerifyJobEndpointResult
}

// handleSetJobEndpoint sets (or replaces) the HTTP endpoint URL for a job,
// validates against SSRF, and generates a fresh HMAC signing secret.
func (s *Server) handleSetJobEndpoint(ctx context.Context, input *SetJobEndpointInput) (*SetJobEndpointOutput, error) {
	if err := s.validate.Struct(&input.Body); err != nil {
		return nil, newValidationError(err)
	}

	if err := s.validateEndpointURL(input.Body.EndpointURL); err != nil {
		slog.Warn("endpoint_url failed SSRF validation",
			"url", httputil.RedactURLForLog(input.Body.EndpointURL),
			"err", err.Error(),
			"actor", actorFromContext(ctx),
			"project_id", projectIDFromContext(ctx),
		)
		return nil, huma.Error400BadRequest("endpoint_url failed validation")
	}
	if input.Body.FallbackEndpointURL != "" {
		if err := s.validateEndpointURL(input.Body.FallbackEndpointURL); err != nil {
			slog.Warn("fallback_endpoint_url failed SSRF validation",
				"url", httputil.RedactURLForLog(input.Body.FallbackEndpointURL),
				"err", err.Error(),
				"actor", actorFromContext(ctx),
				"project_id", projectIDFromContext(ctx),
			)
			return nil, huma.Error400BadRequest("fallback_endpoint_url failed validation")
		}
	}

	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	s.emitInternalSecretBypassAuditIfProjectless(ctx, "set_job_endpoint.project_match", "handleSetJobEndpoint", "job", job.ID)
	if err := s.requireSecretsWriteForSecretBearingEndpointChange(ctx, job, input.Body.EndpointURL, input.Body.FallbackEndpointURL); err != nil {
		return nil, err
	}

	// Default to preserving the existing signing secret. Only generate a fresh
	// one when the caller explicitly opts in via rotate_signing_secret=true.
	signingSecret := job.EndpointSigningSecret
	var plaintextSecret string
	if input.Body.RotateSigningSecret {
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return nil, huma.Error500InternalServerError("failed to generate signing secret")
		}
		signingSecret = "esec_" + hex.EncodeToString(secretBytes)
		plaintextSecret = signingSecret
	}
	signingSecret, err = s.preserveOrEncryptEndpointSigningSecret(signingSecret)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to encrypt endpoint signing secret")
	}

	if err := s.store.UpdateJobEndpoint(ctx, input.JobID, job.ProjectID, input.Body.EndpointURL, input.Body.FallbackEndpointURL, signingSecret); err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to update job endpoint")
	}

	slog.Info("job endpoint set",
		"job_id", input.JobID,
		"endpoint_url_host", urlHost(input.Body.EndpointURL),
		"rotated_signing_secret", input.Body.RotateSigningSecret,
		"actor", actorFromContext(ctx),
		"project_id", projectIDFromContext(ctx),
	)
	s.emitAuditEvent(ctx, domain.AuditActionEndpointSet, "job", input.JobID, map[string]any{
		"job_id":                 input.JobID,
		"endpoint_url_host":      urlHost(input.Body.EndpointURL),
		"rotated_signing_secret": input.Body.RotateSigningSecret,
	})

	updated, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated job")
	}

	return &SetJobEndpointOutput{Body: &SetJobEndpointResponse{
		Job:           updated,
		SigningSecret: plaintextSecret,
	}}, nil
}

func (s *Server) requireSecretsWriteForSecretBearingEndpointChange(ctx context.Context, job *domain.Job, endpointURL, fallbackURL string) error {
	if job == nil {
		return nil
	}
	if job.EndpointURL == endpointURL && job.FallbackEndpointURL == fallbackURL {
		return nil
	}
	if s.hasProjectPermission(ctx, domain.ScopeSecretsWrite) {
		return nil
	}
	secrets, err := s.store.ListJobSecrets(ctx, job.ProjectID, job.ID, job.EnvironmentID, 1, nil)
	if err != nil {
		slog.Error("failed to check job secrets before endpoint change",
			"job_id", job.ID,
			"project_id", job.ProjectID,
			"error", err,
		)
		return huma.Error500InternalServerError("failed to check job secrets")
	}
	if len(secrets) > 0 {
		return huma.Error403Forbidden("changing endpoint for a job with attached secrets requires secrets:write")
	}
	return nil
}

// handleVerifyJobEndpoint sends a signed HMAC test ping to the job's stored
// endpoint URL and returns the outcome. Mirrors handleTestWebhook.
func (s *Server) handleVerifyJobEndpoint(ctx context.Context, input *VerifyJobEndpointInput) (*VerifyJobEndpointOutput, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	s.emitInternalSecretBypassAuditIfProjectless(ctx, "verify_job_endpoint.project_match", "handleVerifyJobEndpoint", "job", job.ID)
	if job.EndpointURL == "" {
		return nil, huma.Error400BadRequest("job has no endpoint_url configured")
	}

	testPayload := []byte(`{"type":"endpoint.test"}`)
	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, job.EndpointURL, bytes.NewReader(testPayload))
	if err != nil {
		return nil, huma.Error400BadRequest(fmt.Sprintf("failed to build request: %s", err.Error()))
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "Strait-Endpoint-Verify/1.0")
	httpReq.Header.Set("X-Strait-Timestamp", ts)
	if job.EndpointSigningSecret != "" {
		signingSecret, err := s.decryptEndpointSigningSecret(job.EndpointSigningSecret)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to decrypt endpoint signing secret")
		}
		httpReq.Header.Set("X-Strait-Signature", worker.SignHTTPDispatch(signingSecret, ts, testPayload))
	}

	// Re-validate the URL on every hop. Without CheckRedirect, the bare client
	// follows 3xx by default — an endpoint that registered as a public host can
	// return 302 to http://169.254.169.254/ (cloud metadata) and exfiltrate
	// IAM credentials. The SSRF guard at registration time only covers the
	// first hop; this one covers redirect targets.
	start := time.Now()
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: httputil.NewExternalTransport(s.config != nil && s.config.AllowPrivateEndpoints),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			if err := s.validateURL(req.URL.String()); err != nil {
				return fmt.Errorf("redirect blocked by ssrf guard: %w", err)
			}
			return nil
		},
	}
	resp, doErr := client.Do(httpReq)
	latencyMs := time.Since(start).Milliseconds()

	success := false
	statusCode := 0
	errMsg := ""

	if doErr != nil {
		errMsg = "connection to endpoint URL failed"
	} else {
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		statusCode = resp.StatusCode
		success = resp.StatusCode >= 200 && resp.StatusCode < 300
	}

	slog.Info("job endpoint verify",
		"job_id", input.JobID,
		"endpoint_url_host", urlHost(job.EndpointURL),
		"success", success,
		"status_code", statusCode,
		"actor", actorFromContext(ctx),
		"project_id", projectIDFromContext(ctx),
	)
	s.emitAuditEvent(ctx, domain.AuditActionEndpointVerified, "job", input.JobID, map[string]any{
		"job_id":            input.JobID,
		"endpoint_url_host": urlHost(job.EndpointURL),
		"success":           success,
	})

	result := VerifyJobEndpointResult{
		Success:    success,
		StatusCode: statusCode,
		LatencyMs:  latencyMs,
		Error:      errMsg,
	}
	return &VerifyJobEndpointOutput{Body: result}, nil
}
