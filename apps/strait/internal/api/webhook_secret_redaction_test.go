package api

import (
	"encoding/json"
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestJob_WebhookSecret_NotSerialized(t *testing.T) {
	t.Parallel()

	job := domain.Job{
		ID:            "job-1",
		ProjectID:     "proj-1",
		Name:          "test-job",
		WebhookSecret: "super-secret-value",
	}

	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	if strings.Contains(string(data), "super-secret-value") {
		t.Error("serialized Job contains webhook_secret value")
	}
	if strings.Contains(string(data), "webhook_secret") {
		t.Error("serialized Job contains webhook_secret key")
	}
}

func TestWebhookSubscription_Secret_NotSerialized(t *testing.T) {
	t.Parallel()

	sub := domain.WebhookSubscription{
		ID:         "sub-1",
		ProjectID:  "proj-1",
		WebhookURL: "https://example.com/webhook",
		EventTypes: []string{"job.completed"},
		Secret:     "whsec_very-secret-token",
		Active:     true,
	}

	data, err := json.Marshal(sub)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	if strings.Contains(string(data), "whsec_very-secret-token") {
		t.Error("serialized WebhookSubscription contains secret value")
	}
	if strings.Contains(string(data), `"secret"`) {
		t.Error("serialized WebhookSubscription contains secret key")
	}
}

func TestJob_WebhookSecret_Adversarial_NestedMarshal(t *testing.T) {
	t.Parallel()

	type envelope struct {
		Data domain.Job `json:"data"`
	}

	e := envelope{
		Data: domain.Job{
			ID:            "job-1",
			ProjectID:     "proj-1",
			Name:          "test-job",
			WebhookSecret: "nested-secret-value",
		},
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	if strings.Contains(string(data), "nested-secret-value") {
		t.Error("nested serialized Job leaks webhook_secret value")
	}
}

func TestJobVersion_WebhookSecret_NotSerialized(t *testing.T) {
	t.Parallel()

	jv := domain.JobVersion{
		ID:            "jv-1",
		JobID:         "job-1",
		WebhookSecret: "version-secret-value",
	}

	data, err := json.Marshal(jv)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	if strings.Contains(string(data), "version-secret-value") {
		t.Error("serialized JobVersion contains webhook_secret value")
	}
	if strings.Contains(string(data), "webhook_secret") {
		t.Error("serialized JobVersion contains webhook_secret key")
	}
}

func TestWebhookSubscription_Secret_Adversarial_SliceMarshal(t *testing.T) {
	t.Parallel()

	subs := []domain.WebhookSubscription{
		{ID: "sub-1", Secret: "secret-1", Active: true},
		{ID: "sub-2", Secret: "secret-2", Active: false},
	}

	data, err := json.Marshal(subs)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	if strings.Contains(string(data), "secret-1") || strings.Contains(string(data), "secret-2") {
		t.Error("serialized WebhookSubscription slice leaks secret values")
	}
}
