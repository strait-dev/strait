package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newAPICommand(state *appState) *cobra.Command {
	var headers []string
	var fields []string

	cmd := &cobra.Command{
		Use:   "api <METHOD> <PATH>",
		Short: "Call raw orchestrator API",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(strings.TrimSpace(args[0]))
			endpoint := strings.TrimSpace(args[1])
			endpointURL, err := url.Parse(endpoint)
			if err != nil {
				return err
			}
			if endpointURL.IsAbs() || endpointURL.Host != "" {
				return fmt.Errorf("absolute URLs are not allowed; pass API path only")
			}
			if !strings.HasPrefix(endpoint, "/") {
				endpoint = "/" + endpoint
			}

			base, err := url.Parse(strings.TrimRight(state.opts.serverURL, "/"))
			if err != nil {
				return err
			}
			full, err := base.Parse(endpoint)
			if err != nil {
				return err
			}

			payload, err := fieldsToJSON(fields)
			if err != nil {
				return err
			}

			var body io.Reader
			if payload != nil {
				raw, marshalErr := json.Marshal(payload)
				if marshalErr != nil {
					return marshalErr
				}
				body = bytes.NewReader(raw)
			}

			req, err := http.NewRequestWithContext(cmd.Context(), method, full.String(), body)
			if err != nil {
				return err
			}
			req.Header.Set("Accept", "application/json")
			if payload != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			if state.opts.apiKey != "" {
				req.Header.Set("Authorization", "Bearer "+state.opts.apiKey)
			}

			for _, header := range headers {
				parts := strings.SplitN(header, ":", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid header %q, expected Key:Value", header)
				}
				req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			}

			resp, err := (&http.Client{Timeout: state.opts.timeout}).Do(req) //nolint:gosec
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			rawResp, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			if len(rawResp) == 0 {
				fmt.Printf("status: %d\n", resp.StatusCode)
				return nil
			}

			var decoded any
			if json.Unmarshal(rawResp, &decoded) == nil {
				if err := printData(state, decoded); err != nil {
					return err
				}
			} else {
				if _, err := os.Stdout.Write(rawResp); err != nil {
					return err
				}
			}

			if resp.StatusCode >= http.StatusBadRequest {
				return fmt.Errorf("request failed with status %d", resp.StatusCode)
			}

			return nil
		},
	}

	cmd.Flags().StringArrayVar(&headers, "header", nil, "additional header as Key:Value")
	cmd.Flags().StringArrayVar(&fields, "field", nil, "JSON body field as key=value")

	return cmd
}

func fieldsToJSON(fields []string) (map[string]any, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(fields))
	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid field %q, expected key=value", field)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("field key cannot be empty")
		}

		var parsed any
		if json.Unmarshal([]byte(value), &parsed) == nil {
			out[key] = parsed
		} else {
			out[key] = value
		}
	}
	return out, nil
}
