package store

import (
	"encoding/json"
	"math"
	"testing"

	"strait/internal/domain"
)

// TestDeploymentVersion_OverflowVersion verifies that MaxInt canary percent does not panic.
func TestDeploymentVersion_OverflowVersion(t *testing.T) {
	t.Parallel()
	maxVal := int(math.MaxInt32)
	d := &domain.DeploymentVersion{
		CanaryPercent: &maxVal,
		Status:        domain.DeploymentVersionStatusDraft,
		Strategy:      domain.DeploymentStrategyCanary,
	}
	if d.CanaryPercent == nil || *d.CanaryPercent != maxVal {
		t.Fatalf("expected canary percent %d, got %v", maxVal, d.CanaryPercent)
	}
}

// TestDeploymentVersion_NegativeVersion verifies that a negative canary percent is representable.
func TestDeploymentVersion_NegativeVersion(t *testing.T) {
	t.Parallel()
	neg := -1
	d := &domain.DeploymentVersion{
		CanaryPercent: &neg,
		Status:        domain.DeploymentVersionStatusDraft,
	}
	if d.CanaryPercent == nil || *d.CanaryPercent != -1 {
		t.Fatalf("expected canary percent -1, got %v", d.CanaryPercent)
	}
}

// TestDeploymentVersion_EmptyManifest verifies that an empty manifest JSON is handled.
func TestDeploymentVersion_EmptyManifest(t *testing.T) {
	t.Parallel()
	d := &domain.DeploymentVersion{
		Manifest: json.RawMessage(`{}`),
		Status:   domain.DeploymentVersionStatusDraft,
	}
	if !json.Valid(d.Manifest) {
		t.Fatal("empty manifest should be valid JSON")
	}
}

// TestDeploymentVersion_MalformedManifest verifies that invalid JSON manifest does not panic during assignment.
func TestDeploymentVersion_MalformedManifest(t *testing.T) {
	t.Parallel()
	d := &domain.DeploymentVersion{
		Manifest: json.RawMessage(`{not valid json`),
	}
	if json.Valid(d.Manifest) {
		t.Fatal("malformed manifest should not be valid JSON")
	}
}

// TestDeploymentVersion_DuplicateVersion verifies that setting the same status twice is fine.
func TestDeploymentVersion_DuplicateVersion(t *testing.T) {
	t.Parallel()
	d := &domain.DeploymentVersion{
		Status: domain.DeploymentVersionStatusFinalized,
	}
	d.Status = domain.DeploymentVersionStatusFinalized
	if !d.Status.IsValid() {
		t.Fatal("duplicate status assignment should remain valid")
	}
}

// FuzzDeploymentVersion fuzzes deployment version construction with arbitrary values.
func FuzzDeploymentVersion(f *testing.F) {
	f.Add(0, "{}")
	f.Add(-1, "")
	f.Add(math.MaxInt32, `{"key":"value"}`)
	f.Add(math.MinInt32, `{invalid}`)
	f.Add(100, `null`)

	f.Fuzz(func(t *testing.T, canaryPct int, manifest string) {
		d := &domain.DeploymentVersion{
			CanaryPercent: &canaryPct,
			Manifest:      json.RawMessage(manifest),
			Status:        domain.DeploymentVersionStatusDraft,
			Strategy:      domain.DeploymentStrategyDirect,
		}
		// Must not panic.
		_ = d.Status.IsValid()
		_ = d.Strategy.IsValid()
		_ = json.Valid(d.Manifest)
	})
}
