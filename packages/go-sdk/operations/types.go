// Package operations provides domain service types for the Strait API.
package operations

import (
	"context"
	"strings"
)

// PathParams replaces {param} placeholders in a path with values.
func PathParams(path string, params map[string]string) string {
	for k, v := range params {
		path = strings.ReplaceAll(path, "{"+k+"}", v)
	}
	return path
}

// Requester abstracts the Client.doRequest method for domain services.
type Requester interface {
	DoRequest(ctx context.Context, method, path string, query map[string]string, headers map[string]string, body any, result any) error
}
