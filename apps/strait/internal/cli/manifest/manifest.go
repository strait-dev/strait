package manifest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

// BuildManifest compiles a ProjectConfig into a deterministic ProjectManifest.
func BuildManifest(cfg *ProjectConfig) *ProjectManifest {
	jobs := make([]JobDefinition, len(cfg.Jobs))
	copy(jobs, cfg.Jobs)
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Slug < jobs[j].Slug
	})

	workflows := make([]WorkflowDefinition, len(cfg.Workflows))
	copy(workflows, cfg.Workflows)
	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].Slug < workflows[j].Slug
	})

	m := &ProjectManifest{
		Version:     1,
		ProjectID:   cfg.Project.ID,
		ProjectName: cfg.Project.Name,
		Runtime:     cfg.Runtime,
		Jobs:        jobs,
		Workflows:   workflows,
	}

	m.Checksum = computeChecksum(m)

	return m
}

// computeChecksum generates a SHA-256 hash of the manifest content
// (excluding the checksum field for determinism).
// The hashInput struct mirrors the manifest fields to ensure JSON tag
// consistency with the source types.
func computeChecksum(m *ProjectManifest) string {
	hashInput := struct {
		Version     int                  `json:"version"`
		ProjectID   string               `json:"project_id"`
		ProjectName string               `json:"project_name,omitempty"`
		Runtime     string               `json:"runtime,omitempty"`
		Jobs        []JobDefinition      `json:"jobs,omitempty"`
		Workflows   []WorkflowDefinition `json:"workflows,omitempty"`
	}{
		Version:     m.Version,
		ProjectID:   m.ProjectID,
		ProjectName: m.ProjectName,
		Runtime:     m.Runtime,
		Jobs:        m.Jobs,
		Workflows:   m.Workflows,
	}

	// json.Marshal cannot fail here: the struct contains only basic types
	// (strings, ints, slices of structs with basic fields). If it ever did
	// fail, the panic is preferable to silently producing an empty checksum
	// that would make two different manifests appear identical.
	raw, err := json.Marshal(hashInput)
	if err != nil {
		panic(fmt.Sprintf("manifest checksum: unexpected marshal error: %v", err))
	}

	sum := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", sum)
}
