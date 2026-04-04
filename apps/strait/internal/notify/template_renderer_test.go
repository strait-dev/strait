package notify

import (
	"testing"

	"strait/internal/domain"
)

func TestRenderTemplate_LocalizedAndHandlebars(t *testing.T) {
	tmpl := &domain.NotificationTemplate{
		TemplateKey:   "job-completed",
		DefaultLocale: "en",
		Channels: []byte(`{
			"email": {"subject": "{{job.name}} completed", "text_body": "Hello {{subscriber.name}}"},
			"inbox": {"title": "{{job.name}} completed", "body": "{{payload.body}}"}
		}`),
		LocaleTemplates: []byte(`{
			"es": {
				"channels": {
					"email": {"subject": "{{job.name}} completado"},
					"inbox": {"title": "{{job.name}} completado"}
				}
			}
		}`),
	}

	ctx := map[string]any{
		"subscriber": map[string]any{"name": "Alice"},
		"job":        map[string]any{"name": "Export CSV"},
		"payload":    map[string]any{"body": "Your file is ready."},
	}

	rendered, err := RenderTemplate(tmpl, "es", ctx)
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}

	email := rendered.Channels["email"].(map[string]any)
	if email["subject"] != "Export CSV completado" {
		t.Fatalf("email.subject = %v, want Export CSV completado", email["subject"])
	}
	if email["text_body"] != "Hello Alice" {
		t.Fatalf("email.text_body = %v, want Hello Alice", email["text_body"])
	}

	inbox := rendered.Channels["inbox"].(map[string]any)
	if inbox["title"] != "Export CSV completado" {
		t.Fatalf("inbox.title = %v, want Export CSV completado", inbox["title"])
	}
	if inbox["body"] != "Your file is ready." {
		t.Fatalf("inbox.body = %v, want Your file is ready.", inbox["body"])
	}
}
