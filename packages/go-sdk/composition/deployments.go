package composition

import (
	"context"
	"errors"
)

// DeploymentClient abstracts deployment API operations for testability.
type DeploymentClient interface {
	CreateDeployment(ctx context.Context, body CreateDeploymentVersionBody) (*DeploymentVersion, error)
	FinalizeDeployment(ctx context.Context, deploymentID string, body DeploymentVersionMutationBody) (*DeploymentVersion, error)
	PromoteDeployment(ctx context.Context, deploymentID string, body DeploymentVersionMutationBody) (*DeploymentVersion, error)
	RollbackDeployment(ctx context.Context, deploymentID string, body DeploymentVersionMutationBody) (*DeploymentVersion, error)
}

// CreateDeploymentVersionBody is the request payload for creating a deployment version.
type CreateDeploymentVersionBody struct {
	ProjectID   string         `json:"project_id"`
	Environment string         `json:"environment"`
	Runtime     string         `json:"runtime"`
	ArtifactURI string         `json:"artifact_uri"`
	Manifest    map[string]any `json:"manifest,omitempty"`
	Checksum    string         `json:"checksum,omitempty"`
}

// DeploymentVersionMutationBody is the request payload for finalize/promote/rollback.
type DeploymentVersionMutationBody struct {
	ProjectID   string `json:"project_id"`
	Environment string `json:"environment"`
}

// DeploymentVersion represents a deployment version response.
type DeploymentVersion struct {
	ID string `json:"id"`
}

// CreateAndFinalizeOutput is the result of CreateAndFinalizeDeployment.
type CreateAndFinalizeOutput struct {
	Created   *DeploymentVersion
	Finalized *DeploymentVersion
}

// CreateFinalizePromoteOutput is the result of CreateFinalizePromoteDeployment.
type CreateFinalizePromoteOutput struct {
	Created   *DeploymentVersion
	Finalized *DeploymentVersion
	Promoted  *DeploymentVersion
}

// CreateAndFinalizeDeployment creates and immediately finalizes a deployment version.
func CreateAndFinalizeDeployment(
	ctx context.Context,
	client DeploymentClient,
	createBody CreateDeploymentVersionBody,
	finalizeBody *DeploymentVersionMutationBody,
) (*CreateAndFinalizeOutput, error) {
	created, err := client.CreateDeployment(ctx, createBody)
	if err != nil {
		return nil, err
	}
	if created.ID == "" {
		return nil, errors.New("deployment response is missing a usable id")
	}

	fb := inferMutationBody(createBody)
	if finalizeBody != nil {
		fb = *finalizeBody
	}

	finalized, err := client.FinalizeDeployment(ctx, created.ID, fb)
	if err != nil {
		return nil, err
	}

	return &CreateAndFinalizeOutput{Created: created, Finalized: finalized}, nil
}

// CreateFinalizePromoteDeployment creates, finalizes, and promotes a deployment version.
func CreateFinalizePromoteDeployment(
	ctx context.Context,
	client DeploymentClient,
	createBody CreateDeploymentVersionBody,
	finalizeBody *DeploymentVersionMutationBody,
	promoteBody *DeploymentVersionMutationBody,
) (*CreateFinalizePromoteOutput, error) {
	out, err := CreateAndFinalizeDeployment(ctx, client, createBody, finalizeBody)
	if err != nil {
		return nil, err
	}

	deploymentID := out.Finalized.ID
	if deploymentID == "" {
		deploymentID = out.Created.ID
	}

	pb := inferMutationBody(createBody)
	if promoteBody != nil {
		pb = *promoteBody
	} else if finalizeBody != nil {
		pb = *finalizeBody
	}

	promoted, err := client.PromoteDeployment(ctx, deploymentID, pb)
	if err != nil {
		return nil, err
	}

	return &CreateFinalizePromoteOutput{
		Created:   out.Created,
		Finalized: out.Finalized,
		Promoted:  promoted,
	}, nil
}

func inferMutationBody(create CreateDeploymentVersionBody) DeploymentVersionMutationBody {
	return DeploymentVersionMutationBody{
		ProjectID:   create.ProjectID,
		Environment: create.Environment,
	}
}
