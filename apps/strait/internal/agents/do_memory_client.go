package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

// validAgentID matches only safe agent IDs (UUIDv7 format).
var validAgentID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// DOMemoryClient proxies agent persistent memory operations to Cloudflare
// Durable Objects via the CF REST API. Each agent definition maps to one DO
// instance keyed by agent ID.
type DOMemoryClient struct {
	accountID string
	namespace string
	apiToken  string
	client    *http.Client
}

// NewDOMemoryClient creates a new Durable Objects memory client.
// Returns nil if accountID or apiToken is empty (self-hosted / CF not configured).
func NewDOMemoryClient(accountID, namespace, apiToken string) *DOMemoryClient {
	if accountID == "" || apiToken == "" {
		return nil
	}
	return &DOMemoryClient{
		accountID: accountID,
		namespace: namespace,
		apiToken:  apiToken,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// DOMemoryEntry represents a memory key-value entry from the DO.
type DOMemoryEntry struct {
	Key          string          `json:"key"`
	Value        json.RawMessage `json:"value"`
	SizeBytes    int             `json:"size_bytes"`
	TTLExpiresAt *int64          `json:"ttl_expires_at,omitempty"`
	CreatedAt    int64           `json:"created_at"`
	UpdatedAt    int64           `json:"updated_at"`
}

// Set writes a memory key to the agent's Durable Object.
func (c *DOMemoryClient) Set(ctx context.Context, agentID, key string, value json.RawMessage, ttlSecs, maxPerKey, maxPerAgent int) (*DOMemoryEntry, error) {
	body, err := json.Marshal(map[string]any{
		"value":         json.RawMessage(value),
		"ttl_secs":      ttlSecs,
		"max_per_key":   maxPerKey,
		"max_per_agent": maxPerAgent,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal set request: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, agentID, "/memory/"+key, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DO memory set failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var entry DOMemoryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return nil, fmt.Errorf("decode set response: %w", err)
	}
	return &entry, nil
}

// Get retrieves a memory key from the agent's Durable Object.
func (c *DOMemoryClient) Get(ctx context.Context, agentID, key string) (*DOMemoryEntry, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, agentID, "/memory/"+key, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("DO memory get failed (status %d)", resp.StatusCode)
	}

	var entry DOMemoryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return nil, fmt.Errorf("decode get response: %w", err)
	}
	return &entry, nil
}

// List retrieves all memory keys from the agent's Durable Object.
func (c *DOMemoryClient) List(ctx context.Context, agentID string) ([]DOMemoryEntry, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, agentID, "/memory", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("DO memory list failed (status %d)", resp.StatusCode)
	}

	var entries []DOMemoryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode list response: %w", err)
	}
	return entries, nil
}

// Delete removes a memory key from the agent's Durable Object.
func (c *DOMemoryClient) Delete(ctx context.Context, agentID, key string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, agentID, "/memory/"+key, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("DO memory delete failed (status %d)", resp.StatusCode)
	}
	return nil
}

// doRequest sends an HTTP request to the agent's Durable Object via CF API.
func (c *DOMemoryClient) doRequest(ctx context.Context, method, agentID, path string, body []byte) (*http.Response, error) {
	// CF Durable Objects are accessed via the dispatch namespace.
	// The URL is: https://api.cloudflare.com/client/v4/accounts/{account_id}/workers/durable_objects/namespaces/{namespace_id}/objects/{object_id}
	// But for Workers-to-DO communication, we use the DO stub directly.
	// For Go server → DO, we route through the agent runtime worker which has the DO binding.
	//
	// For now, this is a placeholder that will be wired when the dispatch worker
	// adds a /memory proxy endpoint. The Go server sends memory operations to
	// the dispatch worker, which forwards them to the DO.
	if !validAgentID.MatchString(agentID) {
		return nil, fmt.Errorf("invalid agent ID: %q", agentID)
	}
	reqURL := fmt.Sprintf("https://%s.%s.workers.dev%s",
		url.PathEscape(agentID), url.PathEscape(c.namespace), path)

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create DO request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	return c.client.Do(req)
}

