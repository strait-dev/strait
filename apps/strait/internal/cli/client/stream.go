package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type RunStreamMessage struct {
	Type      string          `json:"type"`
	EventType string          `json:"event_type,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	JobID     string          `json:"job_id,omitempty"`
	ProjectID string          `json:"project_id,omitempty"`
	Level     string          `json:"level,omitempty"`
	Message   string          `json:"message,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	From      string          `json:"from,omitempty"`
	To        string          `json:"to,omitempty"`
	Error     string          `json:"error,omitempty"`
}

func (c *Client) StreamRunEvents(ctx context.Context, runID string, handle func(RunStreamMessage) error) error {
	fullURL, err := url.Parse(c.baseURL)
	if err != nil {
		return err
	}
	fullURL.Path = path.Join(fullURL.Path, "/v1/runs", runID, "stream")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	done := make(chan struct{})
	defer func() {
		close(done)
		_ = resp.Body.Close()
	}()

	go func() {
		select {
		case <-ctx.Done():
			_ = resp.Body.Close()
		case <-done:
		}
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var apiErr map[string]any
		if err := json.Unmarshal(errBody, &apiErr); err == nil {
			if msg, ok := apiErr["error"].(string); ok && msg != "" {
				return fmt.Errorf("run stream failed (%d): %s", resp.StatusCode, msg)
			}
		}
		return fmt.Errorf("run stream failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var eventName string
	var dataLines []string

	flush := func() error {
		if len(dataLines) == 0 {
			eventName = ""
			return nil
		}

		payload := strings.Join(dataLines, "\n")
		dataLines = nil

		var msg RunStreamMessage
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			return fmt.Errorf("decode run stream event: %w", err)
		}
		if eventName == "error" {
			if msg.Error == "" {
				msg.Error = payload
			}
			return fmt.Errorf("run stream error: %s", msg.Error)
		}

		eventName = ""
		return handle(msg)
	}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				return err
			}
		case strings.HasPrefix(line, ":"):
			continue
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, trimSSEField(line))
		}
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func trimSSEField(line string) string {
	value := strings.TrimPrefix(line, "data:")
	if strings.HasPrefix(value, " ") {
		return value[1:]
	}
	return value
}
