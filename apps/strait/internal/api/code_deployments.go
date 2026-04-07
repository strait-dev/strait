package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/objectstore"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

// presignUploadTTL is how long the presigned upload URL remains valid.
// The CLI must upload the tarball within this window after calling create.
const presignUploadTTL = 15 * time.Minute

// --- Create deployment.

type createCodeDeploymentRequest struct {
	ProjectID string `json:"project_id" validate:"required"`
	JobID     string `json:"job_id" validate:"required"`
	Runtime   string `json:"runtime" validate:"required,oneof=python typescript ruby rust go"`
	// SourceHash is the SHA-256 hex digest of the tarball the CLI will upload.
	// The confirm endpoint verifies the uploaded object matches this hash.
	SourceHash      string `json:"source_hash" validate:"required,len=64"`
	SourceSizeBytes int64  `json:"source_size_bytes" validate:"required,min=1"`
}

type CreateCodeDeploymentInput struct {
	Body createCodeDeploymentRequest
}

type CreateCodeDeploymentOutput struct {
	Body struct {
		Deployment *domain.CodeDeployment `json:"deployment"`
		// UploadURL is a presigned PUT URL valid for 15 minutes.
		// The CLI must PUT the tarball directly to this URL.
		UploadURL string `json:"upload_url"`
	}
}

func (s *Server) handleCreateCodeDeployment(ctx context.Context, input *CreateCodeDeploymentInput) (*CreateCodeDeploymentOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}

	runtime := domain.Runtime(req.Runtime)
	if !runtime.IsValid() {
		return nil, huma.Error400BadRequest(fmt.Sprintf("invalid runtime %q: must be python, typescript, ruby, rust, or go", req.Runtime))
	}

	// Verify the job belongs to this project.
	job, err := s.store.GetJob(ctx, req.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to fetch job")
	}
	if job.ProjectID != req.ProjectID {
		return nil, huma.Error403Forbidden("job does not belong to the authenticated project")
	}

	if s.objectStore == nil {
		return nil, huma.Error503ServiceUnavailable("code-first deployments are not configured on this instance")
	}

	deployment := &domain.CodeDeployment{
		JobID:           req.JobID,
		ProjectID:       req.ProjectID,
		Runtime:         runtime,
		SourceHash:      req.SourceHash,
		SourceSizeBytes: req.SourceSizeBytes,
		SourceURI:       objectstore.DeploymentKey(req.ProjectID, req.JobID, ""), // ID filled below
		Status:          domain.DeploymentStatusPending,
		CreatedBy:       actorFromContext(ctx),
	}

	if err := s.store.CreateCodeDeployment(ctx, deployment); err != nil {
		return nil, huma.Error500InternalServerError("failed to create deployment")
	}

	// Now that we have the ID, set the final source URI.
	deployment.SourceURI = objectstore.DeploymentKey(req.ProjectID, req.JobID, deployment.ID)

	uploadURL, err := s.objectStore.PresignUpload(ctx, deployment.SourceURI, presignUploadTTL)
	if err != nil {
		slog.Error("failed to generate presigned upload URL",
			"deployment_id", deployment.ID,
			"error", err,
		)
		return nil, huma.Error500InternalServerError("failed to generate upload URL")
	}

	out := &CreateCodeDeploymentOutput{}
	out.Body.Deployment = deployment
	out.Body.UploadURL = uploadURL
	return out, nil
}

// --- Confirm deployment (trigger build).

type ConfirmCodeDeploymentInput struct {
	JobID        string `path:"jobID"`
	DeploymentID string `path:"deploymentID"`
	Body         struct {
		ProjectID string `json:"project_id" validate:"required"`
	}
}

type ConfirmCodeDeploymentOutput struct {
	Body *domain.CodeDeployment
}

func (s *Server) handleConfirmCodeDeployment(ctx context.Context, input *ConfirmCodeDeploymentInput) (*ConfirmCodeDeploymentOutput, error) {
	if err := requireProjectMatch(ctx, input.Body.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}

	d, err := s.store.GetCodeDeployment(ctx, input.DeploymentID, input.Body.ProjectID)
	if err != nil {
		if errors.Is(err, store.ErrCodeDeploymentNotFound) {
			return nil, huma.Error404NotFound("deployment not found")
		}
		return nil, huma.Error500InternalServerError("failed to fetch deployment")
	}

	if d.JobID != input.JobID {
		return nil, huma.Error404NotFound("deployment not found")
	}

	if d.Status != domain.DeploymentStatusPending {
		return nil, huma.Error409Conflict(
			fmt.Sprintf("deployment is already in status %q; only pending deployments can be confirmed", d.Status),
		)
	}

	// Verify the object was actually uploaded.
	if s.objectStore != nil {
		if _, err := s.objectStore.HeadObject(ctx, d.SourceURI); err != nil {
			if errors.Is(err, objectstore.ErrObjectNotFound) {
				return nil, huma.Error422UnprocessableEntity("tarball not found at upload URL — ensure the upload completed before confirming")
			}
			return nil, huma.Error500InternalServerError("failed to verify upload")
		}
	}

	// Transition to "building" — the build orchestrator (STR-391) will pick this
	// up and submit it to BuildKit. For now we mark it building so the CLI can
	// poll for completion.
	if err := s.store.UpdateCodeDeploymentStatus(ctx, d.ID, domain.DeploymentStatusBuilding, nil); err != nil {
		return nil, huma.Error500InternalServerError("failed to update deployment status")
	}
	d.Status = domain.DeploymentStatusBuilding

	return &ConfirmCodeDeploymentOutput{Body: d}, nil
}

// --- Get deployment.

type GetCodeDeploymentInput struct {
	JobID        string `path:"jobID"`
	DeploymentID string `path:"deploymentID"`
}

type GetCodeDeploymentOutput struct {
	Body *domain.CodeDeployment
}

func (s *Server) handleGetCodeDeployment(ctx context.Context, input *GetCodeDeploymentInput) (*GetCodeDeploymentOutput, error) {
	projectID := projectIDFromContext(ctx)

	d, err := s.store.GetCodeDeployment(ctx, input.DeploymentID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrCodeDeploymentNotFound) {
			return nil, huma.Error404NotFound("deployment not found")
		}
		return nil, huma.Error500InternalServerError("failed to fetch deployment")
	}
	if d.JobID != input.JobID {
		return nil, huma.Error404NotFound("deployment not found")
	}
	return &GetCodeDeploymentOutput{Body: d}, nil
}

// --- List deployments.

type ListCodeDeploymentsInput struct {
	JobID  string `path:"jobID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

type ListCodeDeploymentsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListCodeDeployments(ctx context.Context, input *ListCodeDeploymentsInput) (*ListCodeDeploymentsOutput, error) {
	projectID := projectIDFromContext(ctx)

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	deployments, err := s.store.ListCodeDeployments(ctx, input.JobID, projectID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list deployments")
	}

	return &ListCodeDeploymentsOutput{Body: paginatedResult(deployments, limit, func(d domain.CodeDeployment) string {
		return d.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

// --- Rollback.

type RollbackCodeDeploymentInput struct {
	JobID        string `path:"jobID"`
	DeploymentID string `path:"deploymentID"`
	Body         struct {
		ProjectID string `json:"project_id" validate:"required"`
	}
}

type RollbackCodeDeploymentOutput struct {
	Body *domain.CodeDeployment
}

func (s *Server) handleRollbackCodeDeployment(ctx context.Context, input *RollbackCodeDeploymentInput) (*RollbackCodeDeploymentOutput, error) {
	if err := requireProjectMatch(ctx, input.Body.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}

	d, err := s.store.GetCodeDeployment(ctx, input.DeploymentID, input.Body.ProjectID)
	if err != nil {
		if errors.Is(err, store.ErrCodeDeploymentNotFound) {
			return nil, huma.Error404NotFound("deployment not found")
		}
		return nil, huma.Error500InternalServerError("failed to fetch deployment")
	}
	if d.JobID != input.JobID {
		return nil, huma.Error404NotFound("deployment not found")
	}

	if d.Status != domain.DeploymentStatusReady {
		return nil, huma.Error409Conflict(
			fmt.Sprintf("cannot roll back to deployment in status %q; only ready deployments can be activated", d.Status),
		)
	}

	if err := s.store.RollbackToDeployment(ctx, input.JobID, input.DeploymentID, input.Body.ProjectID); err != nil {
		if errors.Is(err, store.ErrCodeDeploymentNotFound) {
			return nil, huma.Error404NotFound("deployment not found or not in ready status")
		}
		return nil, huma.Error500InternalServerError("failed to roll back deployment")
	}

	return &RollbackCodeDeploymentOutput{Body: d}, nil
}
