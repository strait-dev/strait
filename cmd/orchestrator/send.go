package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func newSendCommand(state *appState) *cobra.Command {
	var data string

	cmd := &cobra.Command{
		Use:   "send <event-type>",
		Short: "Send raw event payload to orchestrator",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			eventType := strings.TrimSpace(args[0])
			if eventType == "" {
				return fmt.Errorf("event type is required")
			}

			base, err := url.Parse(strings.TrimRight(state.opts.serverURL, "/"))
			if err != nil {
				return err
			}
			target, err := base.Parse("/v1/events")
			if err != nil {
				return err
			}

			payload := map[string]any{"type": eventType}
			if strings.TrimSpace(data) != "" {
				var decoded any
				if err := json.Unmarshal([]byte(data), &decoded); err != nil {
					return fmt.Errorf("invalid --data json: %w", err)
				}
				payload["data"] = decoded
			}

			raw, err := json.Marshal(payload)
			if err != nil {
				return err
			}

			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, target.String(), bytes.NewReader(raw))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			if state.opts.apiKey != "" {
				req.Header.Set("Authorization", "Bearer "+state.opts.apiKey)
			}

			resp, err := (&http.Client{Timeout: state.opts.timeout}).Do(req) //nolint:gosec
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode >= http.StatusBadRequest {
				return fmt.Errorf("send failed with status %d", resp.StatusCode)
			}

			return printData(state, map[string]any{"sent": true, "type": eventType, "status": resp.StatusCode})
		},
	}

	cmd.Flags().StringVar(&data, "data", "", "JSON payload for event data")

	return cmd
}
