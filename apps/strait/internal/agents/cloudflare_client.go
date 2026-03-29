package agents

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

const defaultCloudflareAPIBaseURL = "https://api.cloudflare.com/client/v4"

type cloudflareScriptsClient interface {
	UpsertScript(ctx context.Context, req CloudflareScriptUploadRequest) (*CloudflareScriptUploadResult, error)
	DeleteScript(ctx context.Context, namespace, scriptName string) error
}

type CloudflareScriptUploadRequest struct {
	Namespace         string
	ScriptName        string
	CompatibilityDate string
	OutboundWorker    string
	SandboxPolicy     CloudflareSandboxPolicy
	Tags              []string
	Bindings          []map[string]any
	Source            string
}

type CloudflareScriptUploadResult struct {
	ID                string
	ETag              string
	CompatibilityDate string
	ContentSHA256     string
}

type CloudflareAPIClient struct {
	baseURL    string
	accountID  string
	apiToken   string
	httpClient *http.Client
}

func NewCloudflareAPIClient(cfg CloudflareConfig, opts ...CloudflareProviderOption) *CloudflareAPIClient {
	client := &CloudflareAPIClient{
		baseURL:    defaultCloudflareAPIBaseURL,
		accountID:  cfg.AccountID,
		apiToken:   cfg.APIToken,
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt.applyClient(client)
		}
	}
	return client
}

type cloudflareEnvelope[T any] struct {
	Success bool                     `json:"success"`
	Errors  []cloudflareErrorMessage `json:"errors"`
	Result  T                        `json:"result"`
}

type cloudflareErrorMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cloudflareScriptResponse struct {
	ID                string `json:"id"`
	ETag              string `json:"etag"`
	CompatibilityDate string `json:"compatibility_date"`
}

type CloudflareAPIError struct {
	StatusCode int
	Message    string
	Body       string
}

func (e *CloudflareAPIError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("cloudflare api error: status=%d message=%s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("cloudflare api error: status=%d message=%s body=%s", e.StatusCode, e.Message, e.Body)
}

func (e *CloudflareAPIError) Retryable() bool {
	if e == nil {
		return false
	}
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= http.StatusInternalServerError
}

func (c *CloudflareAPIClient) UpsertScript(ctx context.Context, req CloudflareScriptUploadRequest) (*CloudflareScriptUploadResult, error) {
	if strings.TrimSpace(req.Namespace) == "" {
		return nil, errors.New("cloudflare namespace is required")
	}
	if strings.TrimSpace(req.ScriptName) == "" {
		return nil, errors.New("cloudflare script name is required")
	}
	if strings.TrimSpace(req.CompatibilityDate) == "" {
		return nil, errors.New("cloudflare compatibility date is required")
	}
	if strings.TrimSpace(req.Source) == "" {
		return nil, errors.New("cloudflare worker source is required")
	}

	body, contentType, contentHash, err := buildCloudflareMultipartUpload(req)
	if err != nil {
		return nil, err
	}

	endpoint, err := c.scriptURL(req.Namespace, req.ScriptName)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build cloudflare upload request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)
	httpReq.Header.Set("Content-Type", contentType)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute cloudflare upload request: %w", err)
	}
	defer resp.Body.Close()

	result := &CloudflareScriptUploadResult{ContentSHA256: contentHash}
	if err := decodeCloudflareResponse(resp, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *CloudflareAPIClient) DeleteScript(ctx context.Context, namespace, scriptName string) error {
	endpoint, err := c.scriptURL(namespace, scriptName)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build cloudflare delete request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute cloudflare delete request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return decodeCloudflareError(resp)
}

func (c *CloudflareAPIClient) scriptURL(namespace, scriptName string) (string, error) {
	if strings.TrimSpace(namespace) == "" {
		return "", errors.New("cloudflare namespace is required")
	}
	if strings.TrimSpace(scriptName) == "" {
		return "", errors.New("cloudflare script name is required")
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse cloudflare api base url: %w", err)
	}
	base.Path = path.Join(base.Path, "accounts", c.accountID, "workers", "dispatch", "namespaces", namespace, "scripts", scriptName)
	return base.String(), nil
}

func buildCloudflareMultipartUpload(req CloudflareScriptUploadRequest) ([]byte, string, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	metadata := map[string]any{
		"main_module":        "worker.mjs",
		"compatibility_date": req.CompatibilityDate,
		"bindings":           req.Bindings,
		"tags":               req.Tags,
	}
	if req.SandboxPolicy.Mode != "" {
		metadata["annotations"] = map[string]string{
			"strait_sandbox_mode": string(req.SandboxPolicy.Mode),
		}
	}

	metadataPart, err := writer.CreateFormField("metadata")
	if err != nil {
		return nil, "", "", fmt.Errorf("create cloudflare metadata part: %w", err)
	}
	if err := json.NewEncoder(metadataPart).Encode(metadata); err != nil {
		return nil, "", "", fmt.Errorf("encode cloudflare metadata: %w", err)
	}

	sourcePart, err := writer.CreateFormFile("worker.mjs", "worker.mjs")
	if err != nil {
		return nil, "", "", fmt.Errorf("create cloudflare source part: %w", err)
	}
	if _, err := io.WriteString(sourcePart, req.Source); err != nil {
		return nil, "", "", fmt.Errorf("write cloudflare source part: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, "", "", fmt.Errorf("close cloudflare multipart writer: %w", err)
	}

	sum := sha256.Sum256([]byte(req.Source))
	return body.Bytes(), writer.FormDataContentType(), hex.EncodeToString(sum[:]), nil
}

func decodeCloudflareResponse(resp *http.Response, out *CloudflareScriptUploadResult) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeCloudflareError(resp)
	}

	var envelope cloudflareEnvelope[cloudflareScriptResponse]
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode cloudflare api response: %w", err)
	}
	if !envelope.Success {
		return cloudflareEnvelopeError(resp.StatusCode, envelope.Errors)
	}
	if strings.TrimSpace(envelope.Result.ID) == "" {
		return errors.New("cloudflare api response missing result.id")
	}
	out.ID = envelope.Result.ID
	out.ETag = envelope.Result.ETag
	out.CompatibilityDate = envelope.Result.CompatibilityDate
	return nil
}

func decodeCloudflareError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if len(body) == 0 {
		return &CloudflareAPIError{
			StatusCode: resp.StatusCode,
			Message:    http.StatusText(resp.StatusCode),
		}
	}

	var envelope cloudflareEnvelope[json.RawMessage]
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Errors) > 0 {
		return &CloudflareAPIError{
			StatusCode: resp.StatusCode,
			Message:    joinCloudflareErrors(envelope.Errors),
			Body:       string(body),
		}
	}

	return &CloudflareAPIError{
		StatusCode: resp.StatusCode,
		Message:    http.StatusText(resp.StatusCode),
		Body:       string(body),
	}
}

func cloudflareEnvelopeError(statusCode int, errs []cloudflareErrorMessage) error {
	return &CloudflareAPIError{
		StatusCode: statusCode,
		Message:    joinCloudflareErrors(errs),
	}
}

func joinCloudflareErrors(errs []cloudflareErrorMessage) string {
	if len(errs) == 0 {
		return "cloudflare api request failed"
	}
	parts := make([]string, 0, len(errs))
	for _, item := range errs {
		message := strings.TrimSpace(item.Message)
		if message == "" {
			message = http.StatusText(item.Code)
		}
		if item.Code > 0 {
			parts = append(parts, strconv.Itoa(item.Code)+": "+message)
			continue
		}
		parts = append(parts, message)
	}
	return strings.Join(parts, "; ")
}

func buildCloudflareScriptName(agentID string, deploymentVersion int) string {
	replacer := strings.NewReplacer("-", "")
	return "agent-" + replacer.Replace(agentID) + "-v" + strconv.Itoa(deploymentVersion)
}
