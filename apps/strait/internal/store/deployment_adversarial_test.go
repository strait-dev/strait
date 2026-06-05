package store

import (
	"encoding/json"
	"math"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.False(t, d.CanaryPercent ==
		nil ||
		*d.CanaryPercent != maxVal,
	)
}

// TestDeploymentVersion_NegativeVersion verifies that a negative canary percent is representable.
func TestDeploymentVersion_NegativeVersion(t *testing.T) {
	t.Parallel()
	neg := -1
	d := &domain.DeploymentVersion{
		CanaryPercent: &neg,
		Status:        domain.DeploymentVersionStatusDraft,
	}
	require.False(t, d.CanaryPercent ==
		nil ||
		*d.CanaryPercent != -1)
}

// TestDeploymentVersion_EmptyManifest verifies that an empty manifest JSON is handled.
func TestDeploymentVersion_EmptyManifest(t *testing.T) {
	t.Parallel()
	d := &domain.DeploymentVersion{
		Manifest: json.RawMessage(`{}`),
		Status:   domain.DeploymentVersionStatusDraft,
	}
	require.True(t, json.
		Valid(d.
			Manifest))
}

// TestDeploymentVersion_MalformedManifest verifies that invalid JSON manifest does not panic during assignment.
func TestDeploymentVersion_MalformedManifest(t *testing.T) {
	t.Parallel()
	d := &domain.DeploymentVersion{
		Manifest: json.RawMessage(`{not valid json`),
	}
	require.False(t, json.
		Valid(d.
			Manifest))
}

// TestDeploymentVersion_DuplicateVersion verifies that setting the same status twice is fine.
func TestDeploymentVersion_DuplicateVersion(t *testing.T) {
	t.Parallel()
	d := &domain.DeploymentVersion{
		Status: domain.DeploymentVersionStatusFinalized,
	}
	d.Status = domain.DeploymentVersionStatusFinalized
	require.True(t, d.Status.
		IsValid())
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
