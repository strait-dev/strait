package manifest

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildManifest_SortsJobsBySlug(t *testing.T) {
	t.Parallel()

	cfg := &ProjectConfig{
		Project: ProjectInfo{ID: "proj-1"},
		Jobs: []JobDefinition{
			{Slug: "zebra", Name: "Zebra Job"},
			{Slug: "alpha", Name: "Alpha Job"},
			{Slug: "middle", Name: "Middle Job"},
		},
	}

	m := BuildManifest(cfg)
	if len(m.Jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(m.Jobs))
	}
	if m.Jobs[0].Slug != "alpha" || m.Jobs[1].Slug != "middle" || m.Jobs[2].Slug != "zebra" {
		t.Fatalf("jobs not sorted: %v", m.Jobs)
	}
}

func TestBuildManifest_SortsWorkflowsBySlug(t *testing.T) {
	t.Parallel()

	cfg := &ProjectConfig{
		Project: ProjectInfo{ID: "proj-1"},
		Workflows: []WorkflowDefinition{
			{Slug: "zebra-flow", Name: "Zebra Flow"},
			{Slug: "alpha-flow", Name: "Alpha Flow"},
		},
	}

	m := BuildManifest(cfg)
	if len(m.Workflows) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(m.Workflows))
	}
	if m.Workflows[0].Slug != "alpha-flow" || m.Workflows[1].Slug != "zebra-flow" {
		t.Fatalf("workflows not sorted: %v", m.Workflows)
	}
}

func TestBuildManifest_Checksum_Deterministic(t *testing.T) {
	t.Parallel()

	cfg := &ProjectConfig{
		Project: ProjectInfo{ID: "proj-1", Name: "Test"},
		Runtime: "node",
		Jobs:    []JobDefinition{{Slug: "job-1", Name: "Job 1"}},
	}

	m1 := BuildManifest(cfg)
	m2 := BuildManifest(cfg)

	if m1.Checksum != m2.Checksum {
		t.Fatalf("checksums differ: %s vs %s", m1.Checksum, m2.Checksum)
	}

	raw1, err := json.Marshal(m1)
	if err != nil {
		t.Fatalf("marshal m1: %v", err)
	}
	raw2, err := json.Marshal(m2)
	if err != nil {
		t.Fatalf("marshal m2: %v", err)
	}
	if string(raw1) != string(raw2) {
		t.Fatalf("manifest bytes differ:\n%s\n%s", raw1, raw2)
	}
}

func TestBuildManifest_Checksum_DiffersOnChange(t *testing.T) {
	t.Parallel()

	cfg1 := &ProjectConfig{
		Project: ProjectInfo{ID: "proj-1"},
		Jobs:    []JobDefinition{{Slug: "job-1", Name: "Original"}},
	}
	cfg2 := &ProjectConfig{
		Project: ProjectInfo{ID: "proj-1"},
		Jobs:    []JobDefinition{{Slug: "job-1", Name: "Changed"}},
	}

	m1 := BuildManifest(cfg1)
	m2 := BuildManifest(cfg2)

	if m1.Checksum == m2.Checksum {
		t.Fatal("checksums should differ when job name changes")
	}
}

func TestBuildManifest_Version(t *testing.T) {
	t.Parallel()

	cfg := &ProjectConfig{
		Project: ProjectInfo{ID: "proj-1"},
	}

	m := BuildManifest(cfg)
	if m.Version != 1 {
		t.Fatalf("expected version=1, got %d", m.Version)
	}
}

func TestBuildManifest_DoesNotIncludeGeneratedAt(t *testing.T) {
	t.Parallel()

	cfg := &ProjectConfig{
		Project: ProjectInfo{ID: "proj-1"},
	}

	m := BuildManifest(cfg)
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if strings.Contains(string(raw), "generated_at") {
		t.Fatalf("manifest should not include generated_at: %s", raw)
	}
}
