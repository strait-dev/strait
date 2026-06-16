package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

func main() {
	baseURL := os.Getenv("STRAIT_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.strait.dev"
	}
	apiKey := os.Getenv("STRAIT_API_KEY")
	if apiKey == "" {
		panic("STRAIT_API_KEY is required")
	}

	body, err := json.Marshal(map[string]any{
		"payload": map[string]any{
			"user_id": "user_abc123",
			"email":   "customer@example.com",
		},
	})
	if err != nil {
		panic(err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/jobs/send-welcome-email/trigger", bytes.NewReader(body))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		panic(resp.Status)
	}

	var run struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		panic(err)
	}
	fmt.Println(run.ID, run.Status)
}
