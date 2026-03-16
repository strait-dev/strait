package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL    string `json:"url" validate:"required,url"`
		Secret string `json:"secret,omitempty"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	if err := validateURLWithTLS(req.URL, s.config.WebhookRequireTLS); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid url: "+err.Error())
		return
	}

	testPayload, _ := json.Marshal(map[string]any{
		"type":      "webhook.test",
		"timestamp": time.Now().UTC(),
	})

	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, req.URL, bytes.NewReader(testPayload))
	if err != nil {
		respondError(w, r, http.StatusBadRequest, "failed to create request: "+err.Error())
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "Strait-Webhook-Test/1.0")

	if req.Secret != "" {
		ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
		payload := append([]byte(ts+"."), testPayload...)
		mac := hmac.New(sha256.New, []byte(req.Secret))
		_, _ = mac.Write(payload)
		sig := hex.EncodeToString(mac.Sum(nil))
		httpReq.Header.Set("X-Strait-Timestamp", ts)
		httpReq.Header.Set("X-Strait-Signature", "v1="+sig)
	}

	start := time.Now()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"success":    false,
			"error":      err.Error(),
			"latency_ms": latencyMs,
		})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	respondJSON(w, http.StatusOK, map[string]any{
		"success":       resp.StatusCode >= 200 && resp.StatusCode < 300,
		"status_code":   resp.StatusCode,
		"latency_ms":    latencyMs,
		"response_body": string(body),
	})
}

func (s *Server) handleReplayWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	original, err := s.store.GetWebhookDelivery(r.Context(), id)
	if err != nil {
		respondError(w, r, http.StatusNotFound, "webhook delivery not found")
		return
	}

	// Verify the delivery belongs to the caller's project via its associated job.
	if original.JobID != "" {
		job, jobErr := s.store.GetJob(r.Context(), original.JobID)
		if jobErr != nil || job == nil || job.ProjectID != projectIDFromContext(r.Context()) {
			respondError(w, r, http.StatusNotFound, "webhook delivery not found")
			return
		}
	}

	// Clone the delivery at the store layer so the original payload is preserved.
	replay, err := s.store.ReplayWebhookDelivery(r.Context(), id)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create replay delivery")
		return
	}

	respondJSON(w, http.StatusCreated, replay)
}
