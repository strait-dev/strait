package output

import (
	"strings"
	"testing"
)

func TestRenderJSON(t *testing.T) {
	t.Parallel()

	out, err := RenderToString([]map[string]any{{"id": "run_1", "status": "completed"}}, Options{Format: "json"})
	if err != nil {
		t.Fatalf("render json: %v", err)
	}
	if !strings.Contains(out, "run_1") {
		t.Fatalf("unexpected json output: %s", out)
	}
}

func TestRenderTable(t *testing.T) {
	t.Parallel()

	out, err := RenderToString([]map[string]any{{"id": "job_1", "status": "queued"}}, Options{Format: "table", TTY: true})
	if err != nil {
		t.Fatalf("render table: %v", err)
	}
	if !strings.Contains(out, "id") || !strings.Contains(out, "job_1") {
		t.Fatalf("unexpected table output: %s", out)
	}
}

func TestRenderCSV(t *testing.T) {
	t.Parallel()

	out, err := RenderToString([]map[string]any{{"id": "job_1", "status": "queued"}}, Options{Format: "csv"})
	if err != nil {
		t.Fatalf("render csv: %v", err)
	}
	if !strings.Contains(out, "id,status") {
		t.Fatalf("unexpected csv output: %s", out)
	}
}

func TestRenderGoTemplate(t *testing.T) {
	t.Parallel()

	out, err := RenderToString(map[string]any{"id": "run_9"}, Options{Format: "go-template", Template: "{{.id}}"})
	if err != nil {
		t.Fatalf("render go-template: %v", err)
	}
	if strings.TrimSpace(out) != "run_9" {
		t.Fatalf("unexpected template output: %s", out)
	}
}

func TestRenderJSONPath(t *testing.T) {
	t.Parallel()

	out, err := RenderToString(map[string]any{"data": map[string]any{"id": "job_123"}}, Options{Format: "jsonpath", JSONPath: "$.data.id"})
	if err != nil {
		t.Fatalf("render jsonpath: %v", err)
	}
	if strings.TrimSpace(out) != "job_123" {
		t.Fatalf("unexpected jsonpath output: %s", out)
	}
}
