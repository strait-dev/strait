package composition

import (
	"context"
	"errors"
	"testing"
)

type fakeDeploymentClient struct {
	createFn   func(ctx context.Context, body CreateDeploymentVersionBody) (*DeploymentVersion, error)
	finalizeFn func(ctx context.Context, id string, body DeploymentVersionMutationBody) (*DeploymentVersion, error)
	promoteFn  func(ctx context.Context, id string, body DeploymentVersionMutationBody) (*DeploymentVersion, error)
	rollbackFn func(ctx context.Context, id string, body DeploymentVersionMutationBody) (*DeploymentVersion, error)
}

func (f *fakeDeploymentClient) CreateDeployment(ctx context.Context, body CreateDeploymentVersionBody) (*DeploymentVersion, error) {
	return f.createFn(ctx, body)
}
func (f *fakeDeploymentClient) FinalizeDeployment(ctx context.Context, id string, body DeploymentVersionMutationBody) (*DeploymentVersion, error) {
	return f.finalizeFn(ctx, id, body)
}
func (f *fakeDeploymentClient) PromoteDeployment(ctx context.Context, id string, body DeploymentVersionMutationBody) (*DeploymentVersion, error) {
	return f.promoteFn(ctx, id, body)
}
func (f *fakeDeploymentClient) RollbackDeployment(ctx context.Context, id string, body DeploymentVersionMutationBody) (*DeploymentVersion, error) {
	return f.rollbackFn(ctx, id, body)
}

func TestCreateAndFinalizeDeployment_Success(t *testing.T) {
	client := &fakeDeploymentClient{
		createFn: func(_ context.Context, _ CreateDeploymentVersionBody) (*DeploymentVersion, error) {
			return &DeploymentVersion{ID: "dep_1"}, nil
		},
		finalizeFn: func(_ context.Context, id string, _ DeploymentVersionMutationBody) (*DeploymentVersion, error) {
			if id != "dep_1" {
				t.Errorf("expected dep_1, got %q", id)
			}
			return &DeploymentVersion{ID: "dep_1"}, nil
		},
	}

	out, err := CreateAndFinalizeDeployment(context.Background(), client,
		CreateDeploymentVersionBody{ProjectID: "proj_1", Environment: "prod", Runtime: "node", ArtifactURI: "s3://bucket/app.tar.gz"},
		nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Created.ID != "dep_1" || out.Finalized.ID != "dep_1" {
		t.Error("expected dep_1 for both created and finalized")
	}
}

func TestCreateAndFinalizeDeployment_CreateError(t *testing.T) {
	client := &fakeDeploymentClient{
		createFn: func(_ context.Context, _ CreateDeploymentVersionBody) (*DeploymentVersion, error) {
			return nil, errors.New("create failed")
		},
	}

	_, err := CreateAndFinalizeDeployment(context.Background(), client,
		CreateDeploymentVersionBody{ProjectID: "proj_1", Environment: "prod"},
		nil,
	)

	if err == nil || err.Error() != "create failed" {
		t.Errorf("expected 'create failed', got %v", err)
	}
}

func TestCreateAndFinalizeDeployment_MissingID(t *testing.T) {
	client := &fakeDeploymentClient{
		createFn: func(_ context.Context, _ CreateDeploymentVersionBody) (*DeploymentVersion, error) {
			return &DeploymentVersion{}, nil
		},
	}

	_, err := CreateAndFinalizeDeployment(context.Background(), client,
		CreateDeploymentVersionBody{ProjectID: "proj_1", Environment: "prod"},
		nil,
	)

	if err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestCreateFinalizePromoteDeployment_Success(t *testing.T) {
	client := &fakeDeploymentClient{
		createFn: func(_ context.Context, _ CreateDeploymentVersionBody) (*DeploymentVersion, error) {
			return &DeploymentVersion{ID: "dep_1"}, nil
		},
		finalizeFn: func(_ context.Context, _ string, _ DeploymentVersionMutationBody) (*DeploymentVersion, error) {
			return &DeploymentVersion{ID: "dep_1"}, nil
		},
		promoteFn: func(_ context.Context, id string, _ DeploymentVersionMutationBody) (*DeploymentVersion, error) {
			return &DeploymentVersion{ID: id}, nil
		},
	}

	out, err := CreateFinalizePromoteDeployment(context.Background(), client,
		CreateDeploymentVersionBody{ProjectID: "proj_1", Environment: "prod", Runtime: "node", ArtifactURI: "s3://bucket/app.tar.gz"},
		nil, nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Promoted.ID != "dep_1" {
		t.Errorf("expected promoted dep_1, got %q", out.Promoted.ID)
	}
}

func TestCreateFinalizePromoteDeployment_CustomBodies(t *testing.T) {
	var capturedFinalizeBody, capturedPromoteBody DeploymentVersionMutationBody
	client := &fakeDeploymentClient{
		createFn: func(_ context.Context, _ CreateDeploymentVersionBody) (*DeploymentVersion, error) {
			return &DeploymentVersion{ID: "dep_1"}, nil
		},
		finalizeFn: func(_ context.Context, _ string, body DeploymentVersionMutationBody) (*DeploymentVersion, error) {
			capturedFinalizeBody = body
			return &DeploymentVersion{ID: "dep_1"}, nil
		},
		promoteFn: func(_ context.Context, _ string, body DeploymentVersionMutationBody) (*DeploymentVersion, error) {
			capturedPromoteBody = body
			return &DeploymentVersion{ID: "dep_1"}, nil
		},
	}

	fb := DeploymentVersionMutationBody{ProjectID: "proj_fin", Environment: "staging"}
	pb := DeploymentVersionMutationBody{ProjectID: "proj_prom", Environment: "prod"}

	_, err := CreateFinalizePromoteDeployment(context.Background(), client,
		CreateDeploymentVersionBody{ProjectID: "proj_1", Environment: "dev", Runtime: "node", ArtifactURI: "s3://bucket/app.tar.gz"},
		&fb, &pb,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedFinalizeBody.ProjectID != "proj_fin" {
		t.Errorf("expected custom finalize body, got %v", capturedFinalizeBody)
	}
	if capturedPromoteBody.ProjectID != "proj_prom" {
		t.Errorf("expected custom promote body, got %v", capturedPromoteBody)
	}
}
