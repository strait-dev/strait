package strait

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// configFileSchema is the raw JSON shape of strait.json.
type configFileSchema struct {
	Project *configFileProject `json:"project,omitempty"`
	SDK     *configFileSDK     `json:"sdk,omitempty"`
}

type configFileProject struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type configFileSDK struct {
	BaseURL   string `json:"base_url,omitempty"`
	AuthType  string `json:"auth_type,omitempty"`
	TimeoutMs *int   `json:"timeout_ms,omitempty"`
}

// ConfigFileOption configures config file loading behaviour.
type ConfigFileOption func(*configFileOptions)

type configFileOptions struct {
	path string
	dir  string
}

// WithConfigPath sets an explicit config file path.
func WithConfigPath(path string) ConfigFileOption {
	return func(o *configFileOptions) {
		o.path = path
	}
}

// WithConfigDir sets the directory to search for strait.json.
func WithConfigDir(dir string) ConfigFileOption {
	return func(o *configFileOptions) {
		o.dir = dir
	}
}

func resolveConfigFilePath(opts *configFileOptions) string {
	if opts.path != "" {
		return opts.path
	}
	dir := opts.dir
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "strait.json")
}

// ConfigFromFile reads SDK configuration from a strait.json file,
// layering environment variable overrides on top.
func ConfigFromFile(opts ...ConfigFileOption) (*Config, error) {
	options := &configFileOptions{}
	for _, opt := range opts {
		opt(options)
	}

	filePath := resolveConfigFilePath(options)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", filePath, err)
	}

	var schema configFileSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", filePath, err)
	}

	// Extract sdk.* fields with defaults
	baseURL := ""
	authType := AuthTypeAPIKey
	timeoutMs := 30000

	if schema.SDK != nil {
		if schema.SDK.BaseURL != "" {
			baseURL = schema.SDK.BaseURL
		}
		if schema.SDK.AuthType != "" {
			authType = AuthType(schema.SDK.AuthType)
		}
		if schema.SDK.TimeoutMs != nil {
			timeoutMs = *schema.SDK.TimeoutMs
		}
	}

	// Layer env var overrides on top (env vars always win)
	if v := os.Getenv("STRAIT_BASE_URL"); v != "" {
		baseURL = v
	}
	if v := os.Getenv("STRAIT_AUTH_TYPE"); v != "" {
		authType = AuthType(v)
	}
	if v := os.Getenv("STRAIT_TIMEOUT_MS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return nil, &ValidationError{
				Message: fmt.Sprintf("STRAIT_TIMEOUT_MS must be an integer, got %q", v),
				Issues:  []string{"STRAIT_TIMEOUT_MS is not a valid integer"},
			}
		}
		timeoutMs = parsed
	}

	// Token always comes from env var
	apiKey := os.Getenv("STRAIT_API_KEY")

	return &Config{
		BaseURL: NormalizeBaseURL(baseURL),
		Auth: AuthMode{
			Type:  authType,
			Token: apiKey,
		},
		TimeoutMs: timeoutMs,
	}, nil
}

// ProjectID extracts the project.id from a strait.json file.
// Returns empty string if not found.
func ProjectIDFromFile(opts ...ConfigFileOption) (string, error) {
	options := &configFileOptions{}
	for _, opt := range opts {
		opt(options)
	}

	filePath := resolveConfigFilePath(options)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file %q: %w", filePath, err)
	}

	var schema configFileSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return "", fmt.Errorf("failed to parse config file %q: %w", filePath, err)
	}

	if schema.Project != nil {
		return schema.Project.ID, nil
	}
	return "", nil
}

// NewClientFromFile creates a client from strait.json with optional overrides.
func NewClientFromFile(fileOpts []ConfigFileOption, clientOpts ...Option) (*Client, error) {
	cfg, err := ConfigFromFile(fileOpts...)
	if err != nil {
		return nil, err
	}

	allOpts := []Option{
		WithBaseURL(cfg.BaseURL),
		WithAuth(cfg.Auth),
		WithTimeout(cfg.TimeoutMs),
	}
	if cfg.DefaultHeaders != nil {
		allOpts = append(allOpts, WithDefaultHeaders(cfg.DefaultHeaders))
	}
	allOpts = append(allOpts, clientOpts...)

	return NewClient(allOpts...), nil
}
