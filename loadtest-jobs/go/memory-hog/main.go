// Package main allocates configurable memory to test OOM detection and container limits.
//
// Environment variables:
//   - MEMORY_TARGET_MB: target allocation in MB (default: 512)
//   - HOLD_DURATION_SECS: how long to hold memory (default: 10)
//   - STRAIT_SDK_URL, STRAIT_RUN_ID, STRAIT_RUN_TOKEN: SDK connection
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	targetMB := envInt("MEMORY_TARGET_MB", 512)
	holdSecs := envInt("HOLD_DURATION_SECS", 10)

	sdkURL := os.Getenv("STRAIT_SDK_URL")
	runID := os.Getenv("STRAIT_RUN_ID")
	token := os.Getenv("STRAIT_RUN_TOKEN")

	base := fmt.Sprintf("%s/sdk/v1/runs/%s", sdkURL, runID)

	// Allocate in 10MB chunks, touching every page
	chunkSize := 10 * 1024 * 1024 // 10MB
	numChunks := max(targetMB/10, 1)

	chunks := make([][]byte, 0, numChunks)
	for i := range numChunks {
		chunk := make([]byte, chunkSize)
		// Touch every page to ensure physical allocation
		for j := range chunk {
			chunk[j] = byte(j % 256)
		}
		chunks = append(chunks, chunk)

		// Report progress every 100MB
		if (i+1)%10 == 0 {
			progress := float64(i+1) / float64(numChunks)
			sdkPost(base+"/progress", token, map[string]any{
				"progress": progress,
				"message":  fmt.Sprintf("Allocated %dMB / %dMB", (i+1)*10, targetMB),
			})
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Report allocation complete
	sdkPost(base+"/progress", token, map[string]any{
		"progress": 0.9,
		"message":  fmt.Sprintf("Holding %dMB for %ds", targetMB, holdSecs),
	})

	// Hold memory
	time.Sleep(time.Duration(holdSecs) * time.Second)

	// Prevent GC from collecting chunks
	_ = chunks

	// Report output and complete
	sdkPost(base+"/output", token, map[string]any{
		"output": map[string]any{
			"allocated_mb": targetMB,
			"chunks":       len(chunks),
			"hold_seconds": holdSecs,
		},
	})
	sdkPost(base+"/complete", token, map[string]any{})
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func sdkPost(url, token string, body map[string]any) {
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
