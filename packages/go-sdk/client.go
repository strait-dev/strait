package strait

import (
	"context"
	"net/http"
)

// Client is the main Strait API client.
type Client struct {
	config     Config
	httpClient HTTPDoer
	middleware []Middleware
}

// Option configures the Client.
type Option func(*Client)

// WithBaseURL sets the API base URL.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.config.BaseURL = NormalizeBaseURL(baseURL)
	}
}

// WithBearerToken sets Bearer token authentication.
func WithBearerToken(token string) Option {
	return func(c *Client) {
		c.config.Auth = AuthMode{Type: AuthTypeBearer, Token: token}
	}
}

// WithAPIKey sets API key authentication.
func WithAPIKey(key string) Option {
	return func(c *Client) {
		c.config.Auth = AuthMode{Type: AuthTypeAPIKey, Token: key}
	}
}

// WithRunToken sets run token authentication.
func WithRunToken(token string) Option {
	return func(c *Client) {
		c.config.Auth = AuthMode{Type: AuthTypeRunToken, Token: token}
	}
}

// WithAuth sets the authentication mode directly.
func WithAuth(auth AuthMode) Option {
	return func(c *Client) {
		c.config.Auth = auth
	}
}

// WithDefaultHeaders sets default headers sent with every request.
func WithDefaultHeaders(headers map[string]string) Option {
	return func(c *Client) {
		c.config.DefaultHeaders = headers
	}
}

// WithTimeout sets the client timeout in milliseconds.
func WithTimeout(ms int) Option {
	return func(c *Client) {
		c.config.TimeoutMs = ms
	}
}

// WithHTTPClient sets a custom HTTP client (must implement HTTPDoer).
func WithHTTPClient(doer HTTPDoer) Option {
	return func(c *Client) {
		c.httpClient = doer
	}
}

// WithMiddleware appends middleware hooks to the client.
func WithMiddleware(mw ...Middleware) Option {
	return func(c *Client) {
		c.middleware = append(c.middleware, mw...)
	}
}

// NewClient creates a new Strait API client with the given options.
func NewClient(opts ...Option) *Client {
	c := &Client{
		config: Config{
			TimeoutMs: 30000,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.httpClient == nil {
		transport := wrapTransportWithMiddleware(http.DefaultTransport, c.middleware)
		c.httpClient = &http.Client{Transport: transport}
	}

	return c
}

// NewClientFromEnv creates a client configured from environment variables,
// with optional overrides.
func NewClientFromEnv(opts ...Option) (*Client, error) {
	cfg, err := ConfigFromEnv()
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
	allOpts = append(allOpts, opts...)

	return NewClient(allOpts...), nil
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.config.BaseURL
}

// DoRequest exposes the internal HTTP execution for domain services.
func (c *Client) DoRequest(ctx context.Context, method, path string, query map[string]string, headers map[string]string, body any, result any) error {
	return c.doRequest(ctx, RequestOptions{
		Method:  method,
		Path:    path,
		Query:   query,
		Headers: headers,
		Body:    body,
	}, result)
}
